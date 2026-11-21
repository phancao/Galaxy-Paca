"""SPA-10094 — tệp nhịp của notify-bridge / dock-trigger (healthcheck Swarm).

    uv run --with pytest,redis,"psycopg[binary]",httpx \
        pytest deploy/galaxy/notify-bridge/tests -q
"""

from __future__ import annotations

import os
import pathlib
import subprocess
import sys
import time

import pytest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))

import dock_trigger as dt  # noqa: E402
import main as bridge_lib  # noqa: E402


class _Dung(BaseException):  # vòng run() nuốt Exception
    pass


class _Valkey:
    def __init__(self, batches):
        self.batches = list(batches)

    def xreadgroup(self, *a, **kw):
        if not self.batches:
            raise _Dung()
        b = self.batches.pop(0)
        if isinstance(b, Exception):
            raise b
        return b


def _bridge(batches, cls=bridge_lib.Bridge):
    b = cls(tenant="t1", valkey_url="redis://x", database_url="postgres://x")
    v = _Valkey(batches)
    b._paca = v
    b.paca = lambda: v  # vòng lùi đặt _paca = None để nối lại; giữ Valkey giả
    b.ensure_groups = lambda: None
    return b


def test_run_beats_each_loop_even_when_valkey_errors(monkeypatch):
    monkeypatch.setattr(bridge_lib.time, "sleep", lambda s: None)
    b = _bridge([[], ConnectionError("valkey down"), []])
    b._paca.batches.insert(0, [])  # pending sweep "0"
    b.last_beat = 0.0
    with pytest.raises(_Dung):
        b.run()
    assert b.last_beat > 0.0
    assert b.fresh()


def test_drain_beats_per_entry(monkeypatch):
    b = _bridge([[("s", [("1-0", {}), ("2-0", {})])]])
    seen = []
    b.handle_entry = lambda stream, eid, fields: seen.append(b.last_beat)
    b.last_beat = 0.0
    assert b.drain(">") == 2
    assert all(t > 0.0 for t in seen)


def test_fresh_goes_stale_and_stopping_counts_as_fresh():
    b = _bridge([])
    now = time.monotonic()
    b.last_beat = now - b.stale_after_s - 1
    assert not b.fresh(now)
    b.stopping = True
    assert b.fresh(now)


def test_dock_trigger_stale_window_covers_agent_run(monkeypatch):
    monkeypatch.setenv("DOCK_TRIGGER_TIMEOUT_S", "300")
    t = dt.DockTrigger(tenant="t1")
    assert t.stale_after_s >= 300 + 60
    # bridge mặc định giữ ngưỡng ngắn
    assert bridge_lib.Bridge(tenant="t1").stale_after_s == bridge_lib.HEARTBEAT_STALE_S


def test_heartbeat_loop_writes_only_when_all_fresh(tmp_path, monkeypatch):
    tep = tmp_path / "heartbeat"
    a, b = _bridge([]), _bridge([])
    b.tenant = "t2"
    b.last_beat = time.monotonic() - b.stale_after_s - 5  # t2 treo

    vong = {"n": 0}

    def ngu(_s):
        vong["n"] += 1
        if vong["n"] == 1:
            assert not tep.exists(), "một tenant cũ thì không được ghi nhịp"
            b.beat()
        else:
            a.stopping = True

    monkeypatch.setattr(bridge_lib.time, "sleep", ngu)
    bridge_lib.heartbeat_loop([a, b], every_s=0, path=str(tep))
    assert tep.exists()
    assert time.time() - tep.stat().st_mtime < 5


def test_touch_heartbeat_write_error_is_not_fatal(tmp_path):
    assert bridge_lib.touch_heartbeat(str(tmp_path / "khong-co" / "hb")) is False
    assert bridge_lib.touch_heartbeat(str(tmp_path / "hb")) is True


def test_healthcheck_command_declared_in_env_sync(tmp_path):
    """Lệnh sẽ khai trong swarm-env.json: xanh khi nhịp mới, đỏ khi cũ hơn 120 s."""
    tep = tmp_path / "heartbeat"
    lenh = f"import os,time,sys; sys.exit(time.time()-os.path.getmtime({str(tep)!r}) > 120)"
    tep.write_text("1")
    assert subprocess.run([sys.executable, "-c", lenh]).returncode == 0
    cu = time.time() - 300
    os.utime(tep, (cu, cu))
    assert subprocess.run([sys.executable, "-c", lenh]).returncode == 1
