FROM golang:1.22-alpine AS builder

# gitをインストール（go mod downloadに必要）
RUN apk add --no-cache git

WORKDIR /app

# 依存関係をコピー
COPY go.mod ./
# go.sumが存在する場合のみコピー
COPY go.sum* ./
# 依存関係をダウンロード
RUN go mod download

# ソースコードをコピー
COPY . .

# 依存関係を整理してからビルド
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o /app/main .

# 実行用イメージ
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/main .

EXPOSE 8080

CMD ["./main"]

