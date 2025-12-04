FROM alpine:3.18

# Instalar dependencias
RUN apk add --no-cache curl ca-certificates

# Descargar kubectl FIXED VERSION
ENV KUBECTL_VERSION=v1.30.0
RUN curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" \
    && chmod +x kubectl \
    && mv kubectl /usr/local/bin/kubectl

# Copiar el ejecutable boot
COPY boot /boot

ENTRYPOINT ["/boot"]
