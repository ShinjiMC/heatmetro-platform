#!/bin/sh
echo "--- Iniciando Gestor Dinámico de JDKs ---"

# Si no se define la variable, salimos amigablemente
if [ -z "$JDK_VERSIONS" ]; then
  echo "No se definieron versiones en JDK_VERSIONS. Omitiendo descargas."
  exit 0
fi

# Iterar sobre las versiones separadas por coma
IFS=','
for v in $JDK_VERSIONS; do
  # Limpiar espacios
  v=$(echo "$v" | xargs)
  if [ -z "$v" ]; then continue; fi

  TARGET_DIR="/opt/jdks/java$v"

  # Verificar si ya está descargado o si el usuario lo puso manualmente
  if [ -d "$TARGET_DIR" ] && [ -f "$TARGET_DIR/bin/java" ]; then
    echo "✅ Java $v ya está listo en $TARGET_DIR."
  else
    echo "⬇️ Descargando Java $v..."
    mkdir -p "$TARGET_DIR"

    # Llamada a la API dinámica de Adoptium para Linux x64
    API_URL="https://api.adoptium.net/v3/binary/latest/$v/ga/linux/x64/jdk/hotspot/normal/eclipse"
    
    # Descargar y extraer directamente
    if curl -sL -f "$API_URL" | tar -xz -C "$TARGET_DIR" --strip-components=1; then
       echo "✅ Java $v instalado correctamente."
    else
       echo "❌ Error: Adoptium no tiene Java $v o falló la conexión."
       rm -rf "$TARGET_DIR" # Limpiar la carpeta rota
    fi
  fi
done

echo "--- Todos los JDKs procesados ---"