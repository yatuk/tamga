"""Container health probe: exit 0 when the gRPC server answers.

A server with no model loaded is still healthy as a process; whether a model
is loaded is what the Health RPC reports and what the proxy acts on.

The probe sends the service's own token (TAMGA_CLASSIFIER_TOKEN) when one is
set. With TLS on the port (TAMGA_GRPC_TLS_CERT) it only checks that the port
accepts connections: the probe holds no client certificate.
"""

import os
import socket
import sys

import grpc

from classifier.v1.classifier_pb2 import HealthRequest  # type: ignore[import-untyped]
from classifier.v1.classifier_pb2_grpc import ClassifierServiceStub  # type: ignore[import-untyped]


def main() -> int:
    port = os.environ.get("TAMGA_CLASSIFIER_PORT", "50052")
    token = os.environ.get("TAMGA_CLASSIFIER_TOKEN", "").strip()
    try:
        if os.environ.get("TAMGA_GRPC_TLS_CERT", "").strip():
            socket.create_connection(("localhost", int(port)), timeout=3).close()
            return 0
        metadata = (("authorization", f"Bearer {token}"),) if token else None
        with grpc.insecure_channel(f"localhost:{port}") as channel:
            ClassifierServiceStub(channel).Health(HealthRequest(), timeout=3, metadata=metadata)
    except Exception as exc:
        print(f"unhealthy: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
