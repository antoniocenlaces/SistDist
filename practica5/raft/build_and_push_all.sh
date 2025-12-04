#new
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(pwd)"   # ejecutar desde la raíz del repo (ver instrucciones abajo)
# Rutas relativas esperadas (desde ROOT_DIR)
SRVRAFT_DIR="cmd/srvraft"                 # contiene main.go -> nodoRaft
BOOT_DIR="internal/despliegue/boot"       # contiene boot executable / Dockerfile.boot
CLIENT_DIR="pkg/cltraft"                  # contiene cliente -> Dockerfile.cliente

# Dockerfiles 
DOCKERFILE_NODORAFT="cmd/srvraft/Dockerfile.nodoRaft"
DOCKERFILE_BOOT="internal/despliegue/boot/Dockerfile.boot"
DOCKERFILE_CLIENT="pkg/cltraft/Dockerfile.cliente"

# Imagenes / tags
REGISTRY="localhost:5001"
IMG_NODORAFT="$REGISTRY/nodoraft:latest"
IMG_BOOT="$REGISTRY/boot:latest"
IMG_CLIENT="$REGISTRY/cliente:latest"

# Go build flags (para ejecutar en cluster kind arm64)
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=arm64

echo "Working from root: $ROOT_DIR"
echo "Building nodoRaft (server)..."
pushd "$SRVRAFT_DIR" > /dev/null
  go build -o nodoRaft main.go
  chmod +x nodoRaft
popd > /dev/null

echo "Building boot..."
pushd "$BOOT_DIR" > /dev/null
  go build -o boot main.go
  chmod +x boot
popd > /dev/null

echo "Building cliente..."
pushd "$CLIENT_DIR" > /dev/null
  go build -o cliente cltraft.go
  chmod +x cliente
popd > /dev/null

# Build Docker images
echo "Building Docker image for nodoRaft -> $IMG_NODORAFT"
sudo docker build -f "$DOCKERFILE_NODORAFT" -t "$IMG_NODORAFT" "$SRVRAFT_DIR"

echo "Building Docker image for boot -> $IMG_BOOT"
# Para boot, asegurarse de que Dockerfile.boot coloca kubectl en /usr/local/bin
# Si tu Dockerfile.boot no contiene el mv, puedes ajustar aquí copiando kubectl localmente
sudo docker build -f "$DOCKERFILE_BOOT" -t "$IMG_BOOT" "$BOOT_DIR"

echo "Building Docker image for cliente -> $IMG_CLIENT"
sudo docker build -f "$DOCKERFILE_CLIENT" -t "$IMG_CLIENT" "$CLIENT_DIR"

# Push images to local registry
echo "Pushing $IMG_NODORAFT"
sudo docker push "$IMG_NODORAFT"

echo "Pushing $IMG_BOOT"
sudo docker push "$IMG_BOOT"

echo "Pushing $IMG_CLIENT"
sudo docker push "$IMG_CLIENT"

echo "All images built and pushed to $REGISTRY"

# Optional: if using kind WITHOUT a registry mirror, you can load images into every kind node:
# kind load docker-image "$IMG_NODORAFT" --name your-kind-cluster
# kind load docker-image "$IMG_BOOT" --name your-kind-cluster
# kind load docker-image "$IMG_CLIENT" --name your-kind-cluster

echo "Done."
