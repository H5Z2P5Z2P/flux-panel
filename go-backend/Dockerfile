FROM golang:1.25-alpine AS builder

WORKDIR /app

# 为了加速国内构建，可以配置 GOPROXY，如果需要的话
# ENV GOPROXY=https://goproxy.cn,direct

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 for static binary (pure Go sqlite driver)
RUN CGO_ENABLED=0 GOOS=linux go build -o flux-panel-backend main.go

FROM alpine:latest

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=builder /app/flux-panel-backend .
# 复制默认配置文件（如果存在）
COPY --from=builder /app/config.yaml .

# 创建必要的目录
RUN mkdir -p logs data

EXPOSE 6365

CMD ["./flux-panel-backend"]
