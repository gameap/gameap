# ui builder image
FROM node:24-alpine AS uibuilder

WORKDIR /app

COPY . .

RUN cd /app/web/frontend && npm install && npm run build --if-present


# builder image
FROM golang:1.25 AS builder

WORKDIR /app
COPY . .
COPY --from=uibuilder /app/web/static/dist /app/web/static/dist

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build ./cmd/gameap


# production image
FROM docker

COPY --from=builder /app/gameap /gameap

RUN mkdir -p /var/www/gameap/storage/app \
    && mkdir -p /var/www/gameap/storage/app/certs/client \
    && mkdir -p /var/www/gameap/storage/app/certs/server

ENV DATABASE_DRIVER=sqlite DATABASE_URL=file:/db.sqlite

EXPOSE 8025
CMD [ "/gameap" ]

ENTRYPOINT [ "/gameap" ]