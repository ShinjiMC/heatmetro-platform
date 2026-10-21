FROM alpine:latest

# Instalar curl y tar para descargar y extraer
RUN apk add --no-cache curl tar

WORKDIR /app
COPY download.sh /app/download.sh
RUN chmod +x /app/download.sh

# Ejecutar el script al iniciar el contenedor
CMD ["/app/download.sh"]