"""Bridge/API cùng đọc một môi trường thì phải ra cùng một đích.

Bảng dưới đây là bản sao NGUYÊN VĂN kỳ vọng của
services/api/internal/config/load.go::buildTenants. Nếu một trong hai bên đổi
cách dẫn xuất mà bên kia không đổi, bridge sẽ lắng nghe ở chỗ không ai xuất
bản — thông báo im lặng tắt. Test này là chỗ phát hiện việc đó.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import main as bridge_lib  # noqa: E402

BASE_VALKEY = "redis://valkey:6379/0"
BASE_DB = "postgres://paca:pw@postgres:5432/paca?sslmode=disable"


def _env(monkeypatch, **kw):
    for k in (
        "PACA_TENANTS", "OIDC_TENANT", "PACA_VALKEY_URL", "PACA_DATABASE_URL",
        "PACA_TENANT_REDIS_TMO", "PACA_TENANT_DSN_TMO",
    ):
        monkeypatch.delenv(k, raising=False)
    monkeypatch.setenv("PACA_VALKEY_URL", BASE_VALKEY)
    monkeypatch.setenv("PACA_DATABASE_URL", BASE_DB)
    for k, v in kw.items():
        monkeypatch.setenv(k, v)


def test_primary_keeps_the_values_the_deployment_already_runs_on(monkeypatch):
    _env(monkeypatch, PACA_TENANTS="galaxy,tmo,hdbank", OIDC_TENANT="galaxy")
    got = bridge_lib.tenant_connections()
    assert got[0] == ("galaxy", BASE_VALKEY, BASE_DB)


def test_further_tenants_match_what_the_api_derives(monkeypatch):
    _env(monkeypatch, PACA_TENANTS="galaxy,tmo,hdbank", OIDC_TENANT="galaxy")
    got = bridge_lib.tenant_connections()
    assert got[1] == (
        "tmo",
        "redis://valkey:6379/1",
        "postgres://paca:pw@postgres:5432/paca_tmo?sslmode=disable",
    )
    assert got[2] == (
        "hdbank",
        "redis://valkey:6379/2",
        "postgres://paca:pw@postgres:5432/paca_hdbank?sslmode=disable",
    )


def test_no_two_tenants_share_a_connection(monkeypatch):
    _env(monkeypatch, PACA_TENANTS="galaxy,tmo,hdbank,spaxe", OIDC_TENANT="galaxy")
    got = bridge_lib.tenant_connections()
    assert len({v for _, v, _ in got}) == len(got)
    assert len({d for _, _, d in got}) == len(got)


def test_primary_leads_whatever_order_it_was_written_in(monkeypatch):
    _env(monkeypatch, PACA_TENANTS="tmo,hdbank,galaxy", OIDC_TENANT="galaxy")
    got = bridge_lib.tenant_connections()
    assert got[0][0] == "galaxy"
    assert got[0][1] == BASE_VALKEY


def test_overrides_win(monkeypatch):
    _env(
        monkeypatch,
        PACA_TENANTS="galaxy,tmo",
        OIDC_TENANT="galaxy",
        PACA_TENANT_REDIS_TMO="redis://other:6379/7",
        PACA_TENANT_DSN_TMO="postgres://x:y@z:5432/tasks_tmo",
    )
    got = bridge_lib.tenant_connections()
    assert got[1] == ("tmo", "redis://other:6379/7", "postgres://x:y@z:5432/tasks_tmo")


def test_no_list_means_one_nameless_tenant(monkeypatch):
    _env(monkeypatch)
    assert bridge_lib.tenant_connections() == [("", BASE_VALKEY, BASE_DB)]
