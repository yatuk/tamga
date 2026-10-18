"""The proxy's token for the analyzer's gRPC port, over a real in-process server.

The servicer here is a stand-in: what is tested is the interceptor, which
the real server is built with (app.grpc_server.serve_grpc).
"""

from __future__ import annotations

import asyncio

import pytest

grpc = pytest.importorskip("grpc")

from analyzer.v1.tamga_pb2 import HealthCheckRequest, HealthCheckResponse  # type: ignore[import-untyped]  # noqa: E402
from analyzer.v1.tamga_pb2_grpc import (  # type: ignore[import-untyped]  # noqa: E402
    AnalyzerServiceServicer,
    AnalyzerServiceStub,
    add_AnalyzerServiceServicer_to_server,
)

from app import service_auth  # noqa: E402

TOKEN = "test-analyzer-token-not-a-secret"


class StandIn(AnalyzerServiceServicer):
    async def HealthCheck(self, request, context):  # type: ignore[override]
        return HealthCheckResponse(status="ok", service="tamga-analyzer")


def health_code(server_token: str, metadata=None):
    async def run():
        server = grpc.aio.server(interceptors=[service_auth.TokenInterceptor(server_token)])
        add_AnalyzerServiceServicer_to_server(StandIn(), server)
        port = server.add_insecure_port("127.0.0.1:0")
        await server.start()
        try:
            async with grpc.aio.insecure_channel(f"127.0.0.1:{port}") as channel:
                try:
                    await AnalyzerServiceStub(channel).HealthCheck(HealthCheckRequest(), metadata=metadata)
                except grpc.aio.AioRpcError as err:
                    return err.code()
                return grpc.StatusCode.OK
        finally:
            await server.stop(0)

    return asyncio.run(run())


@pytest.mark.parametrize(
    "metadata, want",
    [
        ((("authorization", f"Bearer {TOKEN}"),), grpc.StatusCode.OK),
        (None, grpc.StatusCode.UNAUTHENTICATED),
        ((("authorization", "Bearer test-other-token"),), grpc.StatusCode.UNAUTHENTICATED),
        ((("authorization", TOKEN),), grpc.StatusCode.UNAUTHENTICATED),
    ],
)
def test_token_is_required_when_configured(metadata, want):
    assert health_code(TOKEN, metadata) == want


def test_no_token_configured_accepts_every_call():
    assert health_code("") == grpc.StatusCode.OK


def test_load_token_refuses_an_empty_one_when_required():
    with pytest.raises(service_auth.ServiceAuthError, match="TAMGA_ANALYZER_TOKEN"):
        service_auth.load_token("TAMGA_ANALYZER_TOKEN", {"TAMGA_REQUIRE_SERVICE_TOKENS": "true"})
    assert service_auth.load_token("TAMGA_ANALYZER_TOKEN", {}) == ""


def test_tls_settings_must_be_complete():
    assert service_auth.server_credentials({}) is None
    with pytest.raises(service_auth.ServiceAuthError):
        service_auth.server_credentials({"TAMGA_GRPC_TLS_KEY": "/x/key.pem"})
