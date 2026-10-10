"""The checks of the LiteLLM end-to-end test.

Run inside the compose network (run.sh does it, in the fake model's
container), because the network is internal and publishes no port.

    check.py wait tamga|litellm    wait until the service answers
    check.py create-key            make a proxy-scope Tamga key and print it
    check.py run                   send requests through LiteLLM and assert

Standard library only. Nothing here is printed from a request or an answer
except status codes and whether a marker was present.
"""

import json
import sys
import time
import urllib.error
import urllib.request

TAMGA = "http://tamga:8443"
LITELLM = "http://litellm:4000"
UPSTREAM = "http://localhost:8080"

# The same values as docker-compose.yml. They open only this test's containers.
ADMIN_KEY = "e2e-litellm-admin-key-opens-only-this-test"
MASTER_KEY = "sk-e2e-litellm-master-key-opens-only-this-test"

# Passes the TCKN checksum and belongs to no one.
TCKN = "10000000146"
EMAIL = "jane.doe@example.com"
MASK = "[email_REDACTED]"


def call(method, url, body=None, headers=None, timeout=30):
    """Returns (status, text). A refusal is a status, not an exception."""
    data = None if body is None else json.dumps(body).encode("utf-8")
    request = urllib.request.Request(url, data=data, method=method, headers=headers or {})
    if data is not None:
        request.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return response.status, response.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as err:
        return err.code, err.read().decode("utf-8", "replace")


def wait(name):
    url = {"tamga": TAMGA + "/health", "litellm": LITELLM + "/health/liveliness"}[name]
    deadline = time.time() + 180
    while time.time() < deadline:
        try:
            status, _ = call("GET", url, timeout=5)
            if status == 200:
                print(name + " is up")
                return
        except OSError:
            pass
        time.sleep(2)
    sys.exit(name + " did not answer on " + url + " within 180 s")


def create_key():
    status, text = call("POST", TAMGA + "/api/v1/apikeys",
                        {"label": "litellm-e2e", "scope": "proxy"},
                        {"X-Tamga-Admin-Key": ADMIN_KEY})
    if status != 201:
        sys.exit("creating a Tamga key: status %d" % status)
    print(json.loads(text)["raw_key"])


def chat(text):
    return call("POST", LITELLM + "/v1/chat/completions",
                {"model": "fake-model", "messages": [{"role": "user", "content": text}]},
                {"Authorization": "Bearer " + MASTER_KEY})


def seen():
    _, text = call("GET", UPSTREAM + "/_seen")
    return json.loads(text)["seen"]


failures = []


def check(name, ok, detail=""):
    print(("PASS  " if ok else "FAIL  ") + name + ("" if ok or not detail else "  (" + detail + ")"))
    if not ok:
        failures.append(name)


def run():
    # 1. A clean request goes through, and reaches the model.
    status, text = chat("What is the capital of France?")
    check("a clean request is answered 200", status == 200, "status %d" % status)
    check("the clean request reached the model", "What is the capital of France?" in seen())

    # 2. A request with a national id is refused by LiteLLM with a 4xx, the
    #    model never sees it, and the refusal does not repeat the number.
    before = len(seen())
    status, text = chat("Musterinin TC kimlik numarasi " + TCKN + " olarak kayitli.")
    check("a request with a TCKN is refused with a 4xx", 400 <= status < 500, "status %d" % status)
    check("the refusal names the finding", "tc_kimlik" in text)
    check("the refusal does not repeat the number", TCKN not in text)
    check("the request with a TCKN did not reach the model", len(seen()) == before,
          "%d requests reached it" % (len(seen()) - before))

    # 3. A request with an address is sent on masked: LiteLLM writes the
    #    texts Tamga returned into the request it forwards.
    status, text = chat("Please write to " + EMAIL + " about the invoice.")
    check("a request with an address is answered 200", status == 200, "status %d" % status)
    last = seen()[-1] if seen() else ""
    check("the model received the marker", MASK in last)
    check("the model did not receive the address", EMAIL not in last)

    # 4. The answer hook: the model's answer holds a national id.
    status, text = chat("e2e-answer-tckn")
    check("an answer with a TCKN is refused with a 4xx", 400 <= status < 500, "status %d" % status)
    check("the refusal of the answer does not repeat the number", TCKN not in text)

    # 5. The answer hook: the model's answer holds an address.
    status, text = chat("e2e-answer-email")
    check("an answer with an address is answered 200", status == 200, "status %d" % status)
    check("the caller received the marker", MASK in text)
    check("the caller did not receive the address", EMAIL not in text)

    if failures:
        sys.exit("%d check(s) failed" % len(failures))
    print("all checks passed")


if __name__ == "__main__":
    command = sys.argv[1] if len(sys.argv) > 1 else ""
    if command == "wait" and len(sys.argv) == 3 and sys.argv[2] in ("tamga", "litellm"):
        wait(sys.argv[2])
    elif command == "create-key":
        create_key()
    elif command == "run":
        run()
    else:
        sys.exit(__doc__)
