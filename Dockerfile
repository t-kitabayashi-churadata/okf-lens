# 開発用イメージ。ホストに Go をインストールしなくてよいようにする。
FROM golang:1.26-bookworm

ENV CGO_ENABLED=0 \
    GOFLAGS=-buildvcs=false

WORKDIR /src
