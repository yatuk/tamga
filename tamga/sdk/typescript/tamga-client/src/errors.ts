/**
 * Error thrown by {@link TamgaClient} methods when the proxy returns a non-2xx
 * status code or a response that cannot be parsed as JSON.
 */
export class TamgaError extends Error {
  /** HTTP status code. */
  statusCode: number;

  /** Raw response body (string). */
  body: string;

  constructor(statusCode: number, body: string) {
    const message =
      body.length > 256 ? `Tamga ${statusCode}: ${body.slice(0, 256)}…` : `Tamga ${statusCode}: ${body}`;
    super(message);
    this.name = 'TamgaError';
    this.statusCode = statusCode;
    this.body = body;
  }
}
