# --- ETAPA 1: Construcción del binario Go ---
FROM golang:1.25.0-bookworm AS builder

WORKDIR /app

# Copiar módulos y descargar dependencias
COPY go.mod go.sum ./
RUN go mod download

# Copiar todo el código fuente
COPY . .

# Compilar el binario desactivando CGO gracias a modernc.org/sqlite
# RUN CGO_ENABLED=0 GOOS=linux go build -o /app/heatmetro-api .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/heatmetro-api ./cmd/api


# --- ETAPA 2: Imagen Final Ligera ---
FROM debian:bookworm-slim

# 1. Instalar dependencias esenciales
# - default-jre-headless: Mínimo necesario para que SonarScanner y Maven arranquen.
# - maven: Herramienta de construcción base.
# - curl, git, unzip: Herramientas utilitarias.
RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    git \
    unzip \
    maven \
    default-jre \
    xvfb \
    fontconfig \
    libfreetype6 \
    libxext6 \
    libxrender1 \
    libxtst6 \
    && rm -rf /var/lib/apt/lists/*

# 2. Instalar la herramienta 'g' (Gestor de Go)
ENV G_HOME="/root/.g"
RUN curl -sL https://github.com/voidint/g/releases/download/v1.8.0/g1.8.0.linux-amd64.tar.gz \
    | tar -xz -C /usr/local/bin

# 3. Instalar SonarScanner CLI
ENV SONAR_SCANNER_VERSION=6.1.0.4477
RUN curl -sSLo /tmp/sonar-scanner.zip https://binaries.sonarsource.com/Distribution/sonar-scanner-cli/sonar-scanner-cli-${SONAR_SCANNER_VERSION}.zip \
    && unzip /tmp/sonar-scanner.zip -d /opt \
    && rm /tmp/sonar-scanner.zip \
    && mv /opt/sonar-scanner-${SONAR_SCANNER_VERSION} /opt/sonar-scanner

# 4. Configurar Variables de Entorno (PATH)
# Agregamos SonarScanner y 'g' al PATH
ENV PATH="/opt/sonar-scanner/bin:${G_HOME}/go/bin:${PATH}"
ENV G_MIRROR="https://go.dev/dl/"
ENV GOTOOLCHAIN=local
ENV GOROOT="${G_HOME}/go"

# 5. Inicializar 'g' con una versión base
RUN g install 1.22.0 && g use 1.22.0

# 6. Preparar el entorno de la aplicación
WORKDIR /app

# Copiar el binario compilado desde la etapa 1
COPY --from=builder /app/heatmetro-api /app/heatmetro-api

# Crear carpetas necesarias
RUN mkdir -p /app/clones /app/data /opt/jdks && chmod -R 777 /app/clones /app/data

ENV CLONES_PATH=/app/clones

EXPOSE 3001

# Ejecutar el binario
CMD ["/app/heatmetro-api"]