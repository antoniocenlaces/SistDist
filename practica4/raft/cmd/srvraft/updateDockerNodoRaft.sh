#!/bin/bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o nodoRaft main.go
chmod +x nodoRaft
echo "Compilado sin fallos"
docker build -f Dockerfile.nodoRaft -t localhost:5001/nodoraft:5000 .
docker push localhost:5001/nodoraft:5000
