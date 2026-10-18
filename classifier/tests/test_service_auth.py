"""The proxy's token for the classifier service, over a real in-process server."""

from __future__ import annotations

import asyncio

import grpc
import pytest

from classifier.v1.classifier_pb2 import ClassifyRequest, HealthRequest
from classifier.v1.classifier_pb2_grpc import ClassifierServiceStub, add_ClassifierServiceServicer_to_server

from app import service_auth
from app.server import ClassifierServicer
from tests.test_server import loaded

TOKEN = "test-classifier-token-not-a-secret"


def call(server_token: str, fn, metadata=None):
    """Start a server with *server_token*, run fn(stub, metadata), stop it."""

    async def run():
        server = grpc.aio.server(interceptors=[service_auth.TokenInterceptor(server_token)])
        add_ClassifierServiceServicer_to_server(ClassifierServicer(loaded()), server)
        port = server.add_insecure_port("127.0.0.1:0")
        await server.start()
        try:
            async with grpc.aio.insecure_channel(f"127.0.0.1:{port}") as channel:
                return await fn(ClassifierServiceStub(channel), metadata)
        finally:
            await server.stop(0)

    return asyncio.run(run())


async def classify_code(stub, metadata):
    try:
        await stub.Classify(ClassifyRequest(texts=["hello"]), metadata=metadata)
    except grpc.aio.AioRpcError as err:
        return err.code()
    return grpc.StatusCode.OK


async def health_code(stub, metadata):
    try:
        await stub.Health(HealthRequest(), metadata=metadata)
    except grpc.aio.AioRpcError as err:
        return err.code()
    return grpc.StatusCode.OK


def bearer(token: str):
    return (("authorization", f"Bearer {token}"),)


@pytest.mark.parametrize(
    "metadata, want",
    [
        (bearer(TOKEN), grpc.StatusCode.OK),
        (None, grpc.StatusCode.UNAUTHENTICATED),
        (bearer("test-other-token"), grpc.StatusCode.UNAUTHENTICATED),
        (bearer(TOKEN[:10]), grpc.StatusCode.UNAUTHENTICATED),
        (bearer(TOKEN + "x"), grpc.StatusCode.UNAUTHENTICATED),
        ((("authorization", TOKEN),), grpc.StatusCode.UNAUTHENTICATED),
        ((("authorization", f"Basic {TOKEN}"),), grpc.StatusCode.UNAUTHENTICATED),
    ],
)
def test_token_is_required_when_configured(metadata, want):
    assert call(TOKEN, classify_code, metadata) == want


def test_every_rpc_of_the_service_needs_it():
    """The service's own Health RPC carries model names; it is not exempt."""
    assert call(TOKEN, health_code) == grpc.StatusCode.UNAUTHENTICATED
    assert call(TOKEN, health_code, bearer(TOKEN)) == grpc.StatusCode.OK


def test_no_token_configured_accepts_every_call():
    assert call("", classify_code) == grpc.StatusCode.OK
    assert call("", classify_code, bearer(TOKEN)) == grpc.StatusCode.OK


def test_load_token():
    assert service_auth.load_token("TAMGA_CLASSIFIER_TOKEN", {}) == ""
    assert service_auth.load_token("TAMGA_CLASSIFIER_TOKEN", {"TAMGA_CLASSIFIER_TOKEN": " t \n"}) == "t"
    with pytest.raises(service_auth.ServiceAuthError, match="TAMGA_CLASSIFIER_TOKEN"):
        service_auth.load_token("TAMGA_CLASSIFIER_TOKEN", {"TAMGA_REQUIRE_SERVICE_TOKENS": "true"})
    with pytest.raises(service_auth.ServiceAuthError):
        service_auth.load_token(
            "TAMGA_CLASSIFIER_TOKEN", {"TAMGA_REQUIRE_SERVICE_TOKENS": "1", "TAMGA_CLASSIFIER_TOKEN": "  "}
        )
    env = {"TAMGA_REQUIRE_SERVICE_TOKENS": "true", "TAMGA_CLASSIFIER_TOKEN": TOKEN}
    assert service_auth.load_token("TAMGA_CLASSIFIER_TOKEN", env) == TOKEN
    assert service_auth.load_token("TAMGA_CLASSIFIER_TOKEN", {"TAMGA_REQUIRE_SERVICE_TOKENS": "false"}) == ""


def test_token_matches():
    assert service_auth.token_matches("Bearer " + TOKEN, TOKEN)
    assert service_auth.token_matches(b"Bearer " + TOKEN.encode(), TOKEN)
    assert service_auth.token_matches("Bearer  " + TOKEN + " ", TOKEN)
    assert not service_auth.token_matches(None, TOKEN)
    assert not service_auth.token_matches("", TOKEN)
    assert not service_auth.token_matches("bearer " + TOKEN, TOKEN)
    assert service_auth.token_matches(None, "")


def test_tls_settings_must_be_complete(tmp_path):
    assert service_auth.server_credentials({}) is None
    with pytest.raises(service_auth.ServiceAuthError):
        service_auth.server_credentials({"TAMGA_GRPC_TLS_CERT": "/x/cert.pem"})
    with pytest.raises(service_auth.ServiceAuthError):
        service_auth.server_credentials({"TAMGA_GRPC_TLS_CLIENT_CA": "/x/ca.pem"})
    with pytest.raises(service_auth.ServiceAuthError, match="gRPC TLS"):
        service_auth.server_credentials(
            {"TAMGA_GRPC_TLS_CERT": str(tmp_path / "missing.pem"), "TAMGA_GRPC_TLS_KEY": str(tmp_path / "missing-key.pem")}
        )


def test_healthcheck_sends_the_token(monkeypatch):
    from app import healthcheck

    async def run(server_token: str, probe_token: str) -> int:
        server = grpc.aio.server(interceptors=[service_auth.TokenInterceptor(server_token)])
        add_ClassifierServiceServicer_to_server(ClassifierServicer(loaded()), server)
        port = server.add_insecure_port("localhost:0")
        await server.start()
        try:
            monkeypatch.setenv("TAMGA_CLASSIFIER_PORT", str(port))
            monkeypatch.setenv("TAMGA_CLASSIFIER_TOKEN", probe_token)
            monkeypatch.delenv("TAMGA_GRPC_TLS_CERT", raising=False)
            return await asyncio.to_thread(healthcheck.main)
        finally:
            await server.stop(0)

    assert asyncio.run(run(TOKEN, TOKEN)) == 0
    assert asyncio.run(run(TOKEN, "")) == 1
    assert asyncio.run(run("", "")) == 0
