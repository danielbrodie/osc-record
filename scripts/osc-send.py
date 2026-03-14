#!/usr/bin/env python3
"""
osc-send.py — minimal OSC message sender for testing osc-record.

Usage:
  python3 scripts/osc-send.py /record/start
  python3 scripts/osc-send.py /record/stop --host 192.168.0.34 --port 8000
  python3 scripts/osc-send.py /hello/felix/
"""

import argparse
import sys
from pythonosc import udp_client

def main():
    parser = argparse.ArgumentParser(description="Send a single OSC message")
    parser.add_argument("address", help="OSC address (e.g. /record/start)")
    parser.add_argument("--host", default="127.0.0.1", help="Target host (default: 127.0.0.1)")
    parser.add_argument("--port", type=int, default=8000, help="Target port (default: 8000)")
    parser.add_argument("args", nargs="*", help="OSC arguments (optional)")
    opts = parser.parse_args()

    client = udp_client.SimpleUDPClient(opts.host, opts.port)
    client.send_message(opts.address, opts.args or [])
    print(f"Sent: {opts.address} {opts.args} → {opts.host}:{opts.port}")

if __name__ == "__main__":
    main()
