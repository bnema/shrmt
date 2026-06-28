#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  record_shield_capture.sh --out FILE [options]

Options:
  --out FILE         Output pcap path (required)
  --iface IFACE      Interface to capture on (default: eno1)
  --host IP          Host/IP filter, repeatable
  --port PORT        TCP port filter, repeatable
  --duration SEC     Stop automatically after N seconds
  --snaplen BYTES    Capture snaplen (default: 0 = full packets)
  --help             Show this help

Defaults:
  ports = 6466,6467,8987

Examples:
  # Passive capture on a box that is already on-path
  ./research/tools/record_shield_capture.sh \
    --iface eno1 \
    --host 192.168.1.16 \
    --host 192.168.1.42 \
    --out /tmp/shield-official.pcap

  # 30-second capture of all SHIELD-related ports
  ./research/tools/record_shield_capture.sh \
    --iface eno1 \
    --duration 30 \
    --out /tmp/shield-ports.pcap
EOF
}

out_file=
iface="eno1"
duration=""
snaplen=0
hosts=()
ports=(6466 6467 8987)

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      out_file=${2:-}
      shift 2
      ;;
    --iface)
      iface=${2:-}
      shift 2
      ;;
    --host)
      hosts+=("${2:-}")
      shift 2
      ;;
    --port)
      ports+=("${2:-}")
      shift 2
      ;;
    --duration)
      duration=${2:-}
      shift 2
      ;;
    --snaplen)
      snaplen=${2:-}
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "$out_file" ]]; then
  echo "--out is required" >&2
  usage >&2
  exit 1
fi

mkdir -p "$(dirname "$out_file")"

unique_ports=()
declare -A seen_ports=()
for port in "${ports[@]}"; do
  [[ -n "$port" ]] || continue
  [[ "$port" =~ ^[0-9]+$ ]] || { echo "Invalid port: $port" >&2; exit 1; }
  [[ -n "${seen_ports[$port]:-}" ]] && continue
  seen_ports[$port]=1
  unique_ports+=("$port")
done
ports=("${unique_ports[@]}")

host_filter=""
if [[ ${#hosts[@]} -gt 0 ]]; then
  host_parts=()
  for host in "${hosts[@]}"; do
    host_parts+=("host $host")
  done
  host_filter="($(IFS=' or '; echo "${host_parts[*]}") )"
fi

port_parts=()
for port in "${ports[@]}"; do
  port_parts+=("tcp port $port")
done
port_filter="($(IFS=' or '; echo "${port_parts[*]}") )"

if [[ -n "$host_filter" ]]; then
  bpf="$host_filter and $port_filter"
else
  bpf="$port_filter"
fi

meta_file="${out_file}.meta"
{
  echo "timestamp=$(date -Iseconds)"
  echo "host=$(hostname)"
  echo "iface=$iface"
  echo "out_file=$out_file"
  echo "bpf=$bpf"
} > "$meta_file"

echo "[*] Writing capture to: $out_file"
echo "[*] Metadata file:      $meta_file"
echo "[*] Interface:          $iface"
echo "[*] Filter:             $bpf"
if [[ -n "$duration" ]]; then
  echo "[*] Duration:           ${duration}s"
fi

cmd=(tcpdump -i "$iface" -nn -s "$snaplen" -U -w "$out_file" "$bpf")
if [[ -n "$duration" ]]; then
  exec timeout "$duration" "${cmd[@]}"
else
  exec "${cmd[@]}"
fi
