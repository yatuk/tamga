FROM node:22-alpine AS builder

WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci --legacy-peer-deps

COPY . .
# A NEXT_PUBLIC_ value is written into the JavaScript at build time; setting
# it on the running container changes nothing. So the address the browser
# calls the proxy at is a build argument.
ARG NEXT_PUBLIC_API_URL=http://localhost:8443
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL
RUN npm run build

FROM node:22-alpine

RUN addgroup -S tamga && adduser -S -G tamga -u 1001 tamga

WORKDIR /app
COPY --from=builder /app/.next/standalone ./
COPY --from=builder /app/.next/static ./.next/static
COPY --from=builder /app/public ./public

RUN chown -R tamga:tamga /app

USER tamga

EXPOSE 3000

ENV PORT=3000
ENV HOSTNAME="0.0.0.0"

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:3000 || exit 1

CMD ["node", "server.js"]
