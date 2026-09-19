#!/usr/bin/env python3
"""Показывает состояние всех путей MediaMTX: готовность и дорожки."""
import json
import urllib.request

API = "http://127.0.0.1:9997/v3/paths/list"

with urllib.request.urlopen(API, timeout=10) as r:
    data = json.load(r)

for p in sorted(data["items"], key=lambda x: x["name"]):
    name = p["name"]
    ready = p["ready"]
    tracks = p.get("tracks") or []
    readers = p.get("readers") or []
    print(f"{name:45s} ready={str(ready):5s} readers={len(readers):2d} tracks={tracks}")
