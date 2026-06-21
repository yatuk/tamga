class TamgaError(Exception):
    """Exception raised when the Tamga proxy returns an HTTP error
    or a non-JSON response body."""

    def __init__(self, status_code: int, body: str):
        self.status_code = status_code
        self.body = body
        super().__init__(f"Tamga API error {status_code}: {body[:200]}")
