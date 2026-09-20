FROM node:22-alpine
RUN apk add --no-cache chromium font-noto
COPY render.mjs /opt/veda/render.mjs
USER 65534:65534
ENTRYPOINT ["node", "/opt/veda/render.mjs"]
