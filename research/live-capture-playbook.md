# SHIELD live-capture playbook

This playbook is for reverse-engineering the **real wake-from-standby path** used by the official NVIDIA SHIELD app.

## Current finding

The standard Android TV Remote v2 path on port `6466` is **not sufficient** to wake this SHIELD from standby.

Tested keycodes over the standard channel:

- `KEYCODE_POWER`
- `KEYCODE_WAKEUP`
- `KEYCODE_HOME`
- `KEYCODE_TV_POWER`
- `KEYCODE_STB_POWER`
- `KEYCODE_AVR_POWER`
- short sequences combining those keys

Observed result each time:

- connection succeeds
- command send succeeds
- reported power state stays `powered=false`

That strongly suggests the wake behavior lives on the NVIDIA-specific service:

- mDNS: `_nv_shield_remote._tcp`
- port: `8987`
- TLS certificate CN pattern: `nvbeyonder/...`

APK strings also point in the same direction:

- `WakeUp`
- `PowerOff`
- `GoHome`
- `TVPower`
- `STBPower`
- `AVRPower`
- `virtualSpecialInputCommand`
- `encodeVISSpecialInputCommand`

## Important reality check

If your laptop is just another client on the same switched LAN, **it cannot passively see unicast traffic between phone and SHIELD**.

So there are only three realistic ways to inspect the official app traffic:

1. capture on the **router / AP / mirrored switch port**
2. put your laptop **in-path** as a relay / proxy
3. do an active **L2 MITM** setup

For now, this repo includes tooling for **(1) capture** and **(2) relay**.

---

## Tools added

### 1. Passive capture helper

File:

- `research/tools/record_shield_capture.sh`

Example:

```bash
./research/tools/record_shield_capture.sh \
  --iface eno1 \
  --host 192.168.1.16 \
  --host <phone-ip> \
  --out /tmp/shield-official.pcap
```

Defaults to ports:

- `6466`
- `6467`
- `8987`

### 2. Pcap analysis helper

File:

- `research/tools/analyze_shield_pcap.py`

Example:

```bash
./research/tools/analyze_shield_pcap.py /tmp/shield-official.pcap
```

It summarizes:

- TCP flows
- bytes and packet counts per direction
- visible TLS record framing
- a short payload timeline

### 3. Raw TCP relay/logger

File:

- `research/tools/tcp_relay.py`

### 4. Relay toggle helper

File:

- `research/tools/shield_relay_toggle.sh`

This convenience wrapper:

- starts/stops the relay
- opens/closes the matching UFW ports
- keeps pid/stdout/stderr state under `/tmp/shield-relay-control`

Example:

```bash
./research/tools/shield_relay_toggle.sh on \
  --target-host 192.168.1.16 \
  --allow-from 192.168.1.0/24

./research/tools/shield_relay_toggle.sh off
```

Example:

```bash
./research/tools/tcp_relay.py \
  --listen-host 0.0.0.0 \
  --target-host 192.168.1.16 \
  --ports 6466,6467,8987 \
  --log-dir /tmp/shield-relay
```

This is a **pass-through relay**, not a TLS-breaking MITM.

It logs per-session:

- raw bytes in each direction
- timestamps
- TLS record summaries
- chunk previews

Session logs land in directories like:

```text
/tmp/shield-relay/20260418-130000-p8987-s0001/
```

with files such as:

- `meta.json`
- `client_to_server.bin`
- `server_to_client.bin`
- `events.log`

---

## Recommended workflow

### Option A — best if you control the router/AP

Run capture on the box that is actually on-path:

```bash
./research/tools/record_shield_capture.sh \
  --iface <router-facing-iface> \
  --host 192.168.1.16 \
  --host <phone-ip> \
  --out /tmp/shield-official.pcap
```

Then on the phone:

- open the official NVIDIA SHIELD app
- press **Power** once or twice
- stop capture

Then analyze:

```bash
./research/tools/analyze_shield_pcap.py /tmp/shield-official.pcap
```

### Option B — put this machine in-path with a relay

Run the relay here:

```bash
./research/tools/tcp_relay.py \
  --listen-host 0.0.0.0 \
  --target-host 192.168.1.16 \
  --ports 6466,6467,8987 \
  --log-dir /tmp/shield-relay
```

Then try to make the phone app talk to this machine instead of the SHIELD itself.

The most promising route is the app's apparent **manual IP pairing** flow hinted by the APK strings.

What to try:

- point the app at this laptop's IP: `192.168.1.18`
- leave the relay forwarding to the real SHIELD: `192.168.1.16`

If that works, the relay will record the sessions without needing switch-port mirroring.

### Option C — active MITM

Not implemented here yet.

Reason:

- full traffic visibility would require either:
  - router/AP capture,
  - ARP spoofing + forwarding,
  - or a successful TLS MITM strategy
- the TLS MITM path may fail if the mobile app pins or custom-validates the SHIELD certificate

If passive capture and relay both fail, the next step is an explicit **ARP MITM** setup.

---

## What we are looking for in the capture

The goal is to identify which port and flow actually causes wake-from-standby.

Priority signals:

1. does the official app hit `8987` when Power is pressed?
2. is there a distinctive burst on `8987` that does not happen for normal Android TV key traffic?
3. do we see a different timing shape for wake vs home/back/etc.?
4. if using the relay, can we correlate the wake press with a unique TLS-record pattern?

Even without decrypting TLS, a relay capture can still tell us:

- which port is actually used
- how many request/response bursts happen
- packet sizes
- ordering across `6466`, `6467`, and `8987`

That is enough to narrow the reverse-engineering target dramatically.

---

## Known constraints

- A plain workstation capture on the same LAN does **not** see phone↔SHIELD unicast traffic.
- The current relay is **TCP pass-through only**.
- The current analysis script handles classic `pcap`, not `pcapng`.
- The wake implementation in `shrmt` should not be changed again until we have either:
  - a capture of the official app wake flow, or
  - a confirmed NVIDIA-specific command path on `8987`
