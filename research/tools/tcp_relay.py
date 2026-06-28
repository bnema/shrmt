#!/usr/bin/env python3
import argparse
import asyncio
import json
import os
import signal
import time
from pathlib import Path

TLS_TYPES = {
    20: "change_cipher_spec",
    21: "alert",
    22: "handshake",
    23: "application_data",
    24: "heartbeat",
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Raw TCP relay/logger for SHIELD traffic")
    parser.add_argument("--listen-host", default="0.0.0.0", help="Host/IP to bind listeners on")
    parser.add_argument("--target-host", required=True, help="Upstream SHIELD host/IP")
    parser.add_argument(
        "--ports",
        default="6466,6467,8987",
        help="Comma-separated list of listen ports to relay to the same upstream ports",
    )
    parser.add_argument("--log-dir", default="/tmp/shield-relay", help="Directory where session logs are written")
    parser.add_argument("--connect-timeout", type=float, default=5.0, help="Upstream connect timeout in seconds")
    parser.add_argument("--hex-preview", type=int, default=32, help="Number of bytes to hex-preview per chunk")
    return parser.parse_args()


def parse_tls_summary(data: bytes) -> str:
    out = []
    idx = 0
    while idx + 5 <= len(data):
        content_type = data[idx]
        version_major = data[idx + 1]
        record_len = int.from_bytes(data[idx + 3:idx + 5], "big")
        if content_type not in TLS_TYPES or version_major != 3:
            break
        total = 5 + record_len
        if idx + total > len(data):
            out.append(f"{TLS_TYPES[content_type]}(partial,{record_len})")
            break
        out.append(f"{TLS_TYPES[content_type]}({record_len})")
        idx += total
    return ", ".join(out) if out else "non-tls-or-fragment"


class Relay:
    def __init__(self, args: argparse.Namespace) -> None:
        self.args = args
        self.log_dir = Path(args.log_dir)
        self.log_dir.mkdir(parents=True, exist_ok=True)
        self.counter = 0
        self.counter_lock = asyncio.Lock()
        self.servers = []

    async def next_session_dir(self, port: int) -> Path:
        async with self.counter_lock:
            self.counter += 1
            session_id = self.counter
        stamp = time.strftime("%Y%m%d-%H%M%S")
        path = self.log_dir / f"{stamp}-p{port}-s{session_id:04d}"
        path.mkdir(parents=True, exist_ok=True)
        return path

    async def start(self) -> None:
        ports = [int(x) for x in self.args.ports.split(",") if x.strip()]
        for port in ports:
            server = await asyncio.start_server(
                lambda r, w, port=port: self.handle_client(port, r, w),
                self.args.listen_host,
                port,
            )
            self.servers.append(server)
            print(f"[*] Listening on {self.args.listen_host}:{port} -> {self.args.target_host}:{port}")

    async def stop(self) -> None:
        for server in self.servers:
            server.close()
            await server.wait_closed()

    async def handle_client(self, port: int, client_reader: asyncio.StreamReader, client_writer: asyncio.StreamWriter) -> None:
        session_dir = await self.next_session_dir(port)
        client_peer = client_writer.get_extra_info("peername")
        print(f"[+] Connection on port {port} from {client_peer}; logs: {session_dir}")

        try:
            upstream_reader, upstream_writer = await asyncio.wait_for(
                asyncio.open_connection(self.args.target_host, port),
                timeout=self.args.connect_timeout,
            )
        except Exception as exc:
            print(f"[!] Upstream connect failed on port {port}: {exc}")
            client_writer.close()
            await client_writer.wait_closed()
            return

        meta = {
            "port": port,
            "target_host": self.args.target_host,
            "client_peer": client_peer,
            "started_at": time.time(),
        }
        (session_dir / "meta.json").write_text(json.dumps(meta, indent=2) + "\n", encoding="utf-8")

        c2s_bin = open(session_dir / "client_to_server.bin", "ab")
        s2c_bin = open(session_dir / "server_to_client.bin", "ab")
        events = open(session_dir / "events.log", "a", encoding="utf-8")

        async def relay(name: str, reader: asyncio.StreamReader, writer: asyncio.StreamWriter, raw_file) -> None:
            try:
                while True:
                    chunk = await reader.read(65536)
                    if not chunk:
                        break
                    raw_file.write(chunk)
                    raw_file.flush()
                    writer.write(chunk)
                    await writer.drain()
                    preview = chunk[: self.args.hex_preview].hex()
                    tls_summary = parse_tls_summary(chunk)
                    line = (
                        f"{time.time():.6f} dir={name} bytes={len(chunk)} tls={tls_summary} preview={preview}\n"
                    )
                    events.write(line)
                    events.flush()
                    print(f"    {name} bytes={len(chunk)} tls={tls_summary}")
            finally:
                try:
                    writer.close()
                    await writer.wait_closed()
                except Exception:
                    pass

        try:
            await asyncio.gather(
                relay("client_to_server", client_reader, upstream_writer, c2s_bin),
                relay("server_to_client", upstream_reader, client_writer, s2c_bin),
            )
        finally:
            c2s_bin.close()
            s2c_bin.close()
            events.close()
            print(f"[-] Connection on port {port} closed")


async def amain() -> int:
    args = parse_args()
    relay = Relay(args)
    await relay.start()

    stop_event = asyncio.Event()

    def _stop(*_args):
        stop_event.set()

    loop = asyncio.get_running_loop()
    for sig in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(sig, _stop)
        except NotImplementedError:
            pass

    await stop_event.wait()
    await relay.stop()
    return 0


def main() -> int:
    return asyncio.run(amain())


if __name__ == "__main__":
    raise SystemExit(main())
