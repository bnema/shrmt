#!/usr/bin/env python3
import argparse
import ipaddress
import os
import socket
import struct
import sys
from collections import Counter, defaultdict
from dataclasses import dataclass

TLS_TYPES = {
    20: "change_cipher_spec",
    21: "alert",
    22: "handshake",
    23: "application_data",
    24: "heartbeat",
}


@dataclass
class Event:
    ts: float
    src: str
    sport: int
    dst: str
    dport: int
    payload_len: int
    tls_summary: str


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Summarize SHIELD-related traffic from a pcap file")
    parser.add_argument("pcap", help="Path to a pcap file produced by tcpdump")
    parser.add_argument("--timeline", type=int, default=60, help="Maximum payload events to print in the timeline")
    return parser.parse_args()


def read_pcap(path: str):
    with open(path, "rb") as f:
        gh = f.read(24)
        if len(gh) != 24:
            raise ValueError("pcap header too short")

        magic = gh[:4]
        if magic == b"\xd4\xc3\xb2\xa1":
            endian = "<"
        elif magic == b"\xa1\xb2\xc3\xd4":
            endian = ">"
        else:
            raise ValueError("unsupported pcap format or pcapng")

        while True:
            ph = f.read(16)
            if not ph:
                return
            if len(ph) != 16:
                raise ValueError("truncated packet header")
            ts_sec, ts_usec, incl_len, _orig_len = struct.unpack(endian + "IIII", ph)
            data = f.read(incl_len)
            if len(data) != incl_len:
                raise ValueError("truncated packet payload")
            yield ts_sec + ts_usec / 1_000_000.0, data


def parse_packet(data: bytes):
    if len(data) < 14:
        return None

    offset = 12
    eth_type = struct.unpack("!H", data[offset:offset + 2])[0]
    l2_end = 14

    while eth_type in (0x8100, 0x88A8):
        if len(data) < l2_end + 4:
            return None
        eth_type = struct.unpack("!H", data[l2_end + 2:l2_end + 4])[0]
        l2_end += 4

    if eth_type == 0x0800:
        return parse_ipv4(data[l2_end:])
    if eth_type == 0x86DD:
        return parse_ipv6(data[l2_end:])
    return None


def parse_ipv4(data: bytes):
    if len(data) < 20:
        return None
    version_ihl = data[0]
    version = version_ihl >> 4
    if version != 4:
        return None
    ihl = (version_ihl & 0x0F) * 4
    if len(data) < ihl:
        return None
    proto = data[9]
    if proto != 6:
        return None
    src = socket.inet_ntoa(data[12:16])
    dst = socket.inet_ntoa(data[16:20])
    total_len = struct.unpack("!H", data[2:4])[0]
    total_len = min(total_len, len(data))
    return parse_tcp(src, dst, data[ihl:total_len])


def parse_ipv6(data: bytes):
    if len(data) < 40:
        return None
    version = data[0] >> 4
    if version != 6:
        return None
    next_header = data[6]
    if next_header != 6:
        return None
    payload_len = struct.unpack("!H", data[4:6])[0]
    src = str(ipaddress.IPv6Address(data[8:24]))
    dst = str(ipaddress.IPv6Address(data[24:40]))
    end = min(40 + payload_len, len(data))
    return parse_tcp(src, dst, data[40:end])


def parse_tcp(src: str, dst: str, data: bytes):
    if len(data) < 20:
        return None
    sport, dport = struct.unpack("!HH", data[:4])
    data_offset = (data[12] >> 4) * 4
    if len(data) < data_offset:
        return None
    payload = data[data_offset:]
    return src, sport, dst, dport, payload


def parse_tls_records(payload: bytes):
    out = []
    idx = 0
    while idx + 5 <= len(payload):
        content_type = payload[idx]
        version_major = payload[idx + 1]
        version_minor = payload[idx + 2]
        record_len = struct.unpack("!H", payload[idx + 3:idx + 5])[0]
        if content_type not in TLS_TYPES or version_major != 3:
            break
        total = 5 + record_len
        if idx + total > len(payload):
            out.append(f"{TLS_TYPES[content_type]}(partial,{record_len})")
            break
        out.append(f"{TLS_TYPES[content_type]}({record_len})")
        idx += total
    return out


def main() -> int:
    args = parse_args()
    if not os.path.exists(args.pcap):
        print(f"pcap not found: {args.pcap}", file=sys.stderr)
        return 1

    flows = defaultdict(lambda: {
        "c2s_packets": 0,
        "s2c_packets": 0,
        "c2s_bytes": 0,
        "s2c_bytes": 0,
        "tls": Counter(),
        "first_ts": None,
        "last_ts": None,
        "first_dir": None,
    })
    events = []

    for ts, raw in read_pcap(args.pcap):
        parsed = parse_packet(raw)
        if not parsed:
            continue
        src, sport, dst, dport, payload = parsed
        if not payload:
            continue

        forward = (src, sport, dst, dport)
        reverse = (dst, dport, src, sport)
        if reverse in flows:
            key = reverse
            direction = "s2c"
        else:
            key = forward
            direction = "c2s"
            if flows[key]["first_dir"] is None:
                flows[key]["first_dir"] = forward

        flow = flows[key]
        flow[f"{direction}_packets"] += 1
        flow[f"{direction}_bytes"] += len(payload)
        flow["first_ts"] = ts if flow["first_ts"] is None else min(flow["first_ts"], ts)
        flow["last_ts"] = ts if flow["last_ts"] is None else max(flow["last_ts"], ts)

        tls_records = parse_tls_records(payload)
        tls_summary = ", ".join(tls_records) if tls_records else "non-tls-or-fragment"
        for record in tls_records:
            flow["tls"][f"{direction}:{record.split('(')[0]}"] += 1

        events.append(Event(ts, src, sport, dst, dport, len(payload), tls_summary))

    if not flows:
        print("No TCP payload packets found.")
        return 0

    print("Flows")
    print("=====")
    for key, flow in sorted(flows.items(), key=lambda item: item[1]["first_ts"] or 0):
        src, sport, dst, dport = key
        start = flow["first_ts"]
        end = flow["last_ts"]
        duration = (end - start) if start is not None and end is not None else 0.0
        print(f"{src}:{sport} -> {dst}:{dport}")
        print(f"  duration:   {duration:.3f}s")
        print(f"  c2s:        {flow['c2s_packets']} packets / {flow['c2s_bytes']} bytes")
        print(f"  s2c:        {flow['s2c_packets']} packets / {flow['s2c_bytes']} bytes")
        if flow["tls"]:
            top = ", ".join(f"{k}={v}" for k, v in flow["tls"].most_common())
            print(f"  tls:        {top}")
        print()

    print("Timeline")
    print("========")
    for event in events[: args.timeline]:
        print(
            f"{event.ts:.6f} {event.src}:{event.sport} -> {event.dst}:{event.dport} "
            f"payload={event.payload_len} {event.tls_summary}"
        )

    remaining = len(events) - args.timeline
    if remaining > 0:
        print(f"... {remaining} more payload events omitted")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
