"""The proxy's token for this service, and optional TLS on its gRPC port.

The operator generates one token per service (``openssl rand -hex 32``) and
gives the same value to the service and to the proxy. The proxy sends it
with every call as ``authorization: Bearer <token>``. With a token set, a
call without the same one is refused with UNAUTHENTICATED; without one,
every call is accepted, as before. With TAMGA_REQUIRE_SERVICE_TOKENS on, the
service does not start without its token.

The standard health service (grpc.health.v1.Health) is never asked for the
token. TLS is TAMGA_GRPC_TLS_CERT and TAMGA_GRPC_TLS_KEY; with
TAMGA_GRPC_TLS_CLIENT_CA as well, a client must present a certificate signed
by that CA.

The classifier has a copy of this module (classifier/app/service_auth.py):
the two services are built from separate directories. Here the token also
guards the HTTP API (app.main), except /health.
"""

from __future__ import annotations

import hashlib
import hmac
import os
from collections.abc import Mapping

import grpc

REQUIRE_ENV = "TAMGA_REQUIRE_SERVICE_TOKENS"
HEALTH_PREFIX = "/grpc.health.v1.Health/"
REFUSED = "missing or wrong service token"


class ServiceAuthError(RuntimeError):
    """A setting that keeps the service from starting."""


def required(environ: Mapping[str, str] | None = None) -> bool:
    env = os.environ if environ is None else environ
    return env.get(REQUIRE_ENV, "").strip().lower() in {"1", "true", "yes", "y", "on"}


def load_token(name: str, environ: Mapping[str, str] | None = None) -> str:
    """The token in the variable *name*, or "" when it is not set.

    Raises ServiceAuthError when it is empty and REQUIRE_ENV is on.
    """
    env = os.environ if environ is None else environ
    token = env.get(name, "").strip()
    if not token and required(env):
        raise ServiceAuthError(
            f"{name} is empty and {REQUIRE_ENV} is on: generate a token with "
            "`openssl rand -hex 32` and give the same value to the service and the proxy"
        )
    return token


def _digest(value: str) -> bytes:
    return hashlib.sha256(value.encode("utf-8")).digest()


def token_matches(header: str | bytes | None, token: str) -> bool:
    """Whether an authorization value carries *token*. An empty token
    accepts anything.

    Hashes are compared, so the comparison takes the same time whatever the
    length of what was sent.
    """
    if not token:
        return True
    if isinstance(header, bytes):
        header = header.decode("utf-8", errors="replace")
    if not header or not header.startswith("Bearer "):
        return False
    return hmac.compare_digest(_digest(header[len("Bearer "):].strip()), _digest(token))


def _refusal(handler: grpc.RpcMethodHandler) -> grpc.RpcMethodHandler:
    async def refuse(_request, context):
        await context.abort(grpc.StatusCode.UNAUTHENTICATED, REFUSED)

    kwargs = {
        "request_deserializer": handler.request_deserializer,
        "response_serializer": handler.response_serializer,
    }
    if handler.unary_unary:
        return grpc.unary_unary_rpc_method_handler(refuse, **kwargs)
    if handler.unary_stream:
        return grpc.unary_stream_rpc_method_handler(refuse, **kwargs)
    if handler.stream_unary:
        return grpc.stream_unary_rpc_method_handler(refuse, **kwargs)
    return grpc.stream_stream_rpc_method_handler(refuse, **kwargs)


class TokenInterceptor(grpc.aio.ServerInterceptor):
    """Refuses calls that do not carry the token."""

    def __init__(self, token: str) -> None:
        self._token = token

    async def intercept_service(self, continuation, handler_call_details):
        handler = await continuation(handler_call_details)
        if handler is None or not self._token or handler_call_details.method.startswith(HEALTH_PREFIX):
            return handler
        for key, value in handler_call_details.invocation_metadata or ():
            if key == "authorization" and token_matches(value, self._token):
                return handler
        return _refusal(handler)


def server_credentials(environ: Mapping[str, str] | None = None) -> grpc.ServerCredentials | None:
    """TLS credentials for the gRPC port, or None for a plain one."""
    env = os.environ if environ is None else environ
    cert = env.get("TAMGA_GRPC_TLS_CERT", "").strip()
    key = env.get("TAMGA_GRPC_TLS_KEY", "").strip()
    client_ca = env.get("TAMGA_GRPC_TLS_CLIENT_CA", "").strip()
    if not cert and not key:
        if client_ca:
            raise ServiceAuthError("TAMGA_GRPC_TLS_CLIENT_CA needs TAMGA_GRPC_TLS_CERT and TAMGA_GRPC_TLS_KEY")
        return None
    if not cert or not key:
        raise ServiceAuthError("TAMGA_GRPC_TLS_CERT and TAMGA_GRPC_TLS_KEY must be set together")
    try:
        with open(cert, "rb") as f:
            cert_pem = f.read()
        with open(key, "rb") as f:
            key_pem = f.read()
        ca_pem = None
        if client_ca:
            with open(client_ca, "rb") as f:
                ca_pem = f.read()
    except OSError as exc:
        raise ServiceAuthError(f"gRPC TLS: {exc}") from exc
    return grpc.ssl_server_credentials(
        [(key_pem, cert_pem)],
        root_certificates=ca_pem,
        require_client_auth=ca_pem is not None,
    )
