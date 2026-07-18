# Supervisor de procesos / Job Scheduler

Supervisor de procesos para Linux escrito en Go. Carga una lista cerrada de trabajos desde JSON, ejecuta y reinicia procesos según una política, conserva stdout/stderr y ofrece un dashboard web para observarlos y controlarlos. La API nunca recibe comandos ni argumentos ejecutables: solo nombres ya registrados en la configuración.

## Arquitectura

```text
Navegador (polling) -> net/http + handlers -> Supervisor -> os/exec
          |                    |                 |
          +---- JSON DTO <-----+                 +-> grupo de procesos Linux
                               |
                               +-> Store -> logs/*.stdout.log
                                          -> logs/*.stderr.log
```

- `cmd/scheduler`: composition root; conecta configuración, logs, supervisor, HTTP y señales.
- `configs`: configuración de ejemplo validada al arrancar.
- `internal/config`: tipos, valores por defecto y validación JSON.
- `internal/supervisor`: estados, backoff, ciclo de vida y sincronización de jobs.
- `internal/logging`: writers concurrentes y lectura acotada de últimas líneas.
- `internal/api`: rutas, handlers, DTO, respuestas y middleware HTTP.
- `web/templates`: página semántica renderizada con `html/template`.
- `web/static`: CSS por capas y JavaScript ES modules.
- `testdata/procs`: procesos pequeños para demostración y pruebas manuales.
- `bin` y `logs`: artefactos generados en ejecución.

El frontend aplica Atomic Design de forma pragmática: indicadores, badges, botones, selectores y spinner son átomos; tarjetas, acciones y toasts son moléculas; sidebar, estadísticas, tabla y consola son organismos. Se agrupan en una página porque separar cada átomo en plantillas Go diminutas añadiría complejidad y riesgo de nombres duplicados sin aportar reutilización real.

## Requisitos e instalación

- Linux (la gestión de grupos de procesos usa señales POSIX).
- Go 1.25 o compatible con la versión declarada en `go.mod`.

En Ubuntu puede instalarse Go desde los repositorios:

```bash
sudo apt update
sudo apt install golang-go
go version
```

Para requerir exactamente Go 1.25, use el archivo oficial de go.dev correspondiente a su arquitectura en vez del paquete de Ubuntu.

## Compilar y ejecutar

Desde la raíz del repositorio:

```bash
mkdir -p bin
go build -o bin/hello ./testdata/procs/hello.go
go build -o bin/fail ./testdata/procs/fail.go
go build -o bin/infinite ./testdata/procs/infinite.go
go build -o bin/scheduler ./cmd/scheduler
./bin/scheduler -config configs/example.json
```

Abra <http://localhost:8090>. El puerto se configura con `address` en `configs/example.json`.

Al arrancar, los jobs configurados se inician automáticamente. El dashboard consulta resumen y procesos cada 2 segundos; los logs del proceso seleccionado, cada 3 segundos. Los intervalos se detienen con la pestaña oculta y no se inicia otra solicitud mientras la anterior siga pendiente. Se eligió polling porque el volumen es pequeño, tolera reconexiones de forma natural y mantiene el backend y la defensa oral más simples que WebSocket.

## Configuración

Cada proceso admite `name`, `command`, `args`, `restart`, `workdir` y opcionalmente `max_retries`. Los valores globales incluyen dirección HTTP, directorio de logs, gracia de apagado y backoff. Los nombres duplicados, rutas en nombres, comandos vacíos y políticas desconocidas se rechazan antes de ejecutar nada.

Políticas de reinicio:

- `never`: no reinicia, tanto si termina bien como si falla.
- `on-failure`: reinicia solamente ante salida no exitosa.
- `always`: reinicia ante cualquier finalización.

Para el reintento `n`, el retraso es:

```text
min(base × factor^(n-1), max)
```

Los estados expuestos son `starting`, `running`, `backoff`, `stopping`, `stopped` y `failed`.

## API

| Método | Ruta | Resultado |
|---|---|---|
| GET | `/` | Dashboard |
| GET | `/api/health` | Salud del servicio |
| GET | `/api/summary` | Contadores agregados |
| GET | `/api/jobs` | Jobs mediante DTO seguro |
| GET | `/api/jobs/{name}` | Detalle de un job |
| GET | `/api/jobs/{name}/logs?limit=200` | Últimas 1–1000 líneas |
| POST | `/api/jobs/{name}/start` | Inicia un job detenido |
| POST | `/api/jobs/{name}/stop` | Detiene un job activo |
| POST | `/api/jobs/{name}/restart` | Reinicia un job |

Ejemplos:

```bash
curl -s http://localhost:8090/api/health
curl -s http://localhost:8090/api/summary
curl -s http://localhost:8090/api/jobs
curl -s 'http://localhost:8090/api/jobs/infinite/logs?limit=50'
curl -X POST http://localhost:8090/api/jobs/infinite/stop
curl -X POST http://localhost:8090/api/jobs/infinite/start
curl -X POST http://localhost:8090/api/jobs/infinite/restart
```

Las consultas correctas devuelven 200; acciones aceptadas, 202; datos inválidos, 400; jobs inexistentes, 404; métodos incorrectos, 405; transiciones inválidas, 409; errores internos, 500. Los errores siempre tienen `error` y `message`.

## Logs y seguridad

`logs/{job}.stdout.log` y `logs/{job}.stderr.log` se crean en append con fecha, stream y mensaje. La API combina cronológicamente los últimos eventos sin cargar deliberadamente archivos completos en memoria; el límite máximo de respuesta es 1000 entradas. Limpiar la consola solo modifica el DOM del navegador.

El servidor limita cuerpo y cabeceras, define timeouts, recupera panics y no expone errores internos. Los nombres se validan contra traversal. La ejecución usa `exec.Command` directamente, nunca `bash -c` con entrada HTTP. `html/template` escapa el HTML inicial y el JavaScript escapa los valores antes de insertarlos.

## Apagado limpio

SIGINT o SIGTERM detienen primero el servidor HTTP y después el supervisor. Cada comando corre en su propio grupo de procesos: se envía SIGTERM al grupo, se espera `grace_period` y solo después se envía SIGKILL. El supervisor espera `cmd.Wait` y sus goroutines antes de finalizar, evitando zombis y descendientes huérfanos.

```bash
kill -TERM "$(pgrep -f 'bin/scheduler')"
```

## Pruebas y validación

```bash
gofmt -w $(find . -name '*.go')
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

Las pruebas cubren backoff, éxito/fallo, las tres políticas, límite de reintentos, cancelación, stop, restart, shutdown, rutas API, JSON, métodos, códigos y acciones.

## Decisiones y limitaciones

- Un mutex por job protege snapshots breves; un único bucle goroutine posee la ejecución y los reintentos. Los canales se usan solo para esperar `cmd.Wait` y finalización.
- Los DTO desacoplan HTTP de las estructuras internas.
- La persistencia son archivos append; no se necesita base de datos.
- El frontend no requiere CDN, Node ni conexión a Internet.
- SIGHUP/reload dinámico no se implementa: reconciliar jobs añadidos, eliminados y modificados necesita una política explícita. Reinicie el servicio tras cambiar JSON.
- No hay autenticación ni TLS integrados. Para exponerlo fuera de localhost debe colocarse detrás de un reverse proxy seguro y limitar acceso de red.
- Los archivos de log no rotan automáticamente; producción debería añadir rotación por tamaño/tiempo.

## Servicio supervisado: orders-api

`orders-api` fue migrado a un proyecto Go independiente en `/home/geordy/orders-api`. Administra pedidos en memoria mediante HTTP en `http://localhost:9091`; jobscheduler ya no compila ni importa su código fuente.

### Arquitectura

```text
/home/geordy/orders-api/cmd/orders-api/main.go
        |
        +-> transport/http -> service -> repository -> model
              net/http       reglas    RWMutex      Order
```

- `/home/geordy/orders-api/internal/ordersapi`: modelo, persistencia, servicio y transporte HTTP.
- `/home/geordy/orders-api/cmd/orders-api`: composición, configuración, señales y apagado ordenado.

El repositorio usa `sync.RWMutex`: múltiples lecturas pueden avanzar juntas, mientras creación, eliminación y actualización son exclusivas. Los IDs se asignan dentro del mismo bloqueo y por eso son seguros y crecientes. Los pedidos desaparecen cuando el proceso termina, porque esta primera versión almacena todo en memoria.

### Compilar y ejecutar

Desde el proyecto independiente:

```bash
cd ~/orders-api
mkdir -p bin
go build -o bin/orders-api ./cmd/orders-api
./bin/orders-api
```

El servicio queda disponible en <http://localhost:9091>. Para detenerlo correctamente, presione `Ctrl+C`; SIGINT y SIGTERM esperan hasta cinco segundos por solicitudes activas.

### Endpoints

| Método | Ruta | Función |
|---|---|---|
| GET | `/health` | Estado, PID, hora y uptime |
| GET | `/api/orders` | Lista de pedidos |
| POST | `/api/orders` | Crea un pedido |
| GET | `/api/orders/{id}` | Consulta un pedido |
| PATCH | `/api/orders/{id}/status` | Cambia su estado |
| DELETE | `/api/orders/{id}` | Elimina un pedido |
| GET | `/api/orders/stats` | Estadísticas acumuladas |
| POST | `/demo/crash` | Caída controlada de demostración |

Los estados permitidos son `pending`, `processing`, `completed` y `cancelled`. El servidor calcula `total = quantity × unit_price`; cualquier campo `total` enviado por el cliente se rechaza como desconocido.

### Ejemplos

```bash
curl http://localhost:9091/health

curl -X POST http://localhost:9091/api/orders \
  -H "Content-Type: application/json" \
  -d '{
    "customer":"Juan Pérez",
    "product":"Guitarra eléctrica",
    "quantity":2,
    "unit_price":850.50
  }'

curl http://localhost:9091/api/orders
curl http://localhost:9091/api/orders/1
curl http://localhost:9091/api/orders/stats

curl -X PATCH http://localhost:9091/api/orders/1/status \
  -H "Content-Type: application/json" \
  -d '{"status":"completed"}'

curl -X DELETE http://localhost:9091/api/orders/1
```

### Caída controlada

Ejecute esta llamada únicamente después de las demás pruebas:

```bash
curl -X POST http://localhost:9091/demo/crash
```

La API responde primero con HTTP 500 y después de 500 ms termina con código `1`. Esto permitirá demostrar que el supervisor detecta el fallo y aplica backoff/reinicio.

> **Advertencia:** `/demo/crash` existe solo para demostración académica. No debe habilitarse en producción porque cualquier cliente con acceso puede terminar el servicio.

### Pruebas

```bash
gofmt -w $(find . -name '*.go')
go vet ./...
go test ./...
go test -race ./...
go build ./...
go build -o bin/orders-api ./cmd/orders-api
```

Estas pruebas se ejecutan dentro de `~/orders-api`; cubren repositorio concurrente, validaciones, CRUD HTTP, JSON, CORS y crash inyectable.

## Servicios externos supervisados

`orders-api` vive en `/home/geordy/orders-api` y se comunica con los demás programas solamente mediante HTTP/JSON. El ejecutable configurado es `/home/geordy/orders-api/bin/orders-api`; se usa una ruta absoluta porque JSON y `os/exec` no expanden `~`.

```bash
cd ~/orders-api
go build -o bin/orders-api ./cmd/orders-api

cd ~/jobscheduler
go build -o bin/scheduler ./cmd/scheduler
./bin/scheduler -config configs/example.json
```

No inicie `orders-api` manualmente al mismo tiempo que el scheduler: ambas instancias intentarían escuchar en 9091. Desde el dashboard puede iniciar, detener, reiniciar y observar PID, reintentos y logs del servicio externo.

## Proceso supervisado: orders-worker

`orders-worker` es un proceso de segundo plano que consume exclusivamente `orders-api`: consulta pedidos, reclama los `pending` cambiándolos a `processing`, simula trabajo y los completa. No guarda pedidos ni ofrece interfaz de usuario.

```text
cmd/orders-worker -> worker -> client HTTP -> orders-api :9091
                         |-> processor (pending -> processing -> completed)
                         |-> health administrativo 127.0.0.1:9093
```

Los paquetes de `internal/ordersworker` separan configuración, cliente, processor, ciclo concurrente y health. Un channel con capacidad limita el paralelismo y un mapa protegido por mutex impide dos goroutines activas para el mismo ID. Al apagar, el context cancela trabajo activo y un `WaitGroup` espera todas las goroutines; un pedido interrumpido puede quedar `processing` para no declarar falsamente que terminó.

Estados internos: `starting`, `running`, `processing`, `degraded`, `stopping`, `stopped` y `failed`. `/health` devuelve 200 conectado y 503 en `degraded`; `/metrics` expone procesados, fallidos, activos, fallos de API y reconexiones.

| Variable | Predeterminado |
|---|---|
| `ORDERS_API_URL` | `http://localhost:9091` |
| `ORDERS_WORKER_INTERVAL` | `5s` |
| `ORDERS_WORKER_PROCESSING_TIME` | `3s` |
| `ORDERS_WORKER_MAX_CONCURRENT` | `2` |
| `ORDERS_WORKER_REQUEST_TIMEOUT` | `5s` |
| `ORDERS_WORKER_BACKOFF_BASE` | `1s` |
| `ORDERS_WORKER_BACKOFF_MAX` | `30s` |
| `ORDERS_WORKER_HEALTH_ADDR` | `127.0.0.1:9093` |
| `ORDERS_WORKER_SHUTDOWN_TIMEOUT` | `5s` |

Cuando la API falla, el worker sigue vivo y reintenta con `min(base × 2^(intento-1), max)`. Este backoff recupera una dependencia caída; el backoff del supervisor es diferente y solo se aplica si el proceso worker termina.

### Compilar, ejecutar y consultar

```bash
go build -o bin/orders-worker ./cmd/orders-worker
./bin/orders-worker

curl http://127.0.0.1:9093/health
curl http://127.0.0.1:9093/metrics
```

SIGINT y SIGTERM realizan apagado ordenado. La configuración `configs/example.json` registra `orders-worker` con `restart: "on-failure"`: una parada normal no reinicia, pero un crash sí.

```bash
go build -o bin/scheduler ./cmd/scheduler
./bin/scheduler -config configs/example.json
```

Desde el dashboard en <http://localhost:8090> puede iniciarse, detenerse, reiniciarse y consultar logs del worker.

Para demostrar recuperación del supervisor:

```bash
curl -X POST http://127.0.0.1:9093/demo/crash
```

La ruta responde HTTP 500, espera 500 ms y termina con código 1. **Es solo académica y no debe exponerse en producción.** El supervisor detecta el fallo, aplica su propio backoff e inicia otro PID.

Pruebas:

```bash
gofmt -w $(find . -name '*.go')
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

## Aplicación supervisada: orders-web

### Objetivo

`orders-web` es la interfaz web del sistema de pedidos. Es un programa independiente de `orders-api`: no comparte proceso, ni memoria, ni código Go con él. Toda la información que muestra (estado de conexión, estadísticas, listado, detalle) se obtiene en tiempo real por HTTP; `orders-web` no persiste pedidos.

### Arquitectura

```text
cmd/orders-web/main.go   (composition root)
        |
        +-> internal/ordersweb/config    -> lee ORDERS_WEB_ADDR / ORDERS_API_URL
        +-> internal/ordersweb/client    -> cliente HTTP hacia orders-api
        +-> internal/ordersweb/server    -> ServeMux propio, proxy y middleware
              |
              +-> web/orders/templates   -> dashboard.html (html/template)
              +-> web/orders/static      -> css/ y js/ (ES modules, sin build step)
```

- `internal/ordersweb/config`: valores por defecto, variables de entorno y validación de `ORDERS_API_URL`.
- `internal/ordersweb/client`: cliente HTTP reutilizable hacia `orders-api` (`Health`, `ListOrders`, `CreateOrder`, `GetOrder`, `UpdateOrderStatus`, `DeleteOrder`, `GetStats`), con timeout propio, límite de tamaño de respuesta y distinción entre error de conectividad (`ErrUnavailable`, `ErrTimeout`, `ErrBadGateway`) y error de negocio (`APIError`, que ya trae el código y mensaje que devolvió `orders-api`).
- `internal/ordersweb/server`: `server.go` construye el `http.Server` y carga las plantillas; `routes.go` registra el `ServeMux` propio; `handlers.go` implementa el dashboard, `/health` y el proxy; `middleware.go` añade request ID, logging, recuperación de panics y cabeceras de seguridad.
- `web/orders/templates/dashboard.html`: una sola plantilla para toda la página. Con `html/template`, cada archivo cargado por `ParseGlob` debe registrar nombres únicos; separar cada átomo (botón, badge, input) en su propio archivo obligaría a nombrarlos todos sin que ninguno se reutilice fuera de esta página, así que se agrupan como bloques de marcado dentro de un único archivo, igual que ya hace `web/templates/dashboard.html` del propio scheduler.
- `web/orders/static/js`: seis módulos ES sin bundler ni Node — `api.js` (fetch + timeout con `AbortController`), `state.js` (pedidos, filtros, conexión), `orders.js` (tabla, detalle, cambio de estado, eliminación), `forms.js` (validación y total estimado), `notifications.js` (toasts y confirmación) y `dashboard.js` (inicialización, polling, eventos globales).

### Atomic Design

Igual que el dashboard del scheduler, los átomos (botón, badge, input, spinner, indicador de conexión) y las moléculas (tarjeta estadística, campo de formulario, alerta, buscador) se agrupan como bloques de marcado dentro de `dashboard.html` en vez de un archivo por elemento. Los organismos sí quedan separados visualmente por `<section>`/`<aside>`/`<header>`/`<dialog>` con nombres de clase explícitos (`sidebar`, `topbar`, `stats-grid`, `orders-table-body`, `order-detail-modal`, `confirm-modal`) para que la correspondencia entre HTML, CSS y JavaScript sea fácil de señalar en la defensa oral.

### Relación con orders-api

`orders-web` **nunca** almacena pedidos. Cada vista se reconstruye desde `orders-api` en cada solicitud o en cada ciclo de polling. Si `orders-api` no responde, `orders-web` sigue sirviendo el dashboard y su propio `/health`, pero deshabilita el registro y las acciones sobre pedidos hasta que la conexión se recupera.

### Puertos

| Servicio | Puerto |
|---|---|
| `orders-web` | `9092` |
| `orders-api` | `9091` |

### Variables de entorno

| Variable | Predeterminado | Descripción |
|---|---|---|
| `ORDERS_WEB_ADDR` | `:9092` | Dirección de escucha de orders-web |
| `ORDERS_API_URL` | `http://localhost:9091` | Base URL de orders-api; debe ser `http://` o `https://` |

El timeout del cliente HTTP (5 s), la gracia de apagado (5 s) y el intervalo de polling sugerido al dashboard (5 s) tienen valores por defecto fijados en `internal/ordersweb/config`; no se leen variables de entorno adicionales para mantener la configuración mínima pedida en esta fase.

### Cómo compilar

```bash
mkdir -p bin
go build -o bin/orders-web ./cmd/orders-web
```

### Cómo ejecutar

```bash
# En una terminal, orders-api ya debe estar corriendo en :9091
./bin/orders-web
```

Abra <http://localhost:9092>.

### Cómo probar

```bash
gofmt -l internal/ordersweb cmd/orders-web
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

Las pruebas cubren: configuración (valores por defecto, URL válida/ inválida), cliente HTTP (los siete métodos, error HTTP con formato reconocible, JSON inválido, timeout, servidor inalcanzable, respuesta demasiado grande), servidor (dashboard, estáticos, `/health`, cada ruta `/proxy/*` con y sin `orders-api` disponible, método no permitido, tipo de contenido, cabeceras de seguridad, recuperación de panic, `/demo/crash` con función de terminación inyectada) y concurrencia (solicitudes simultáneas al proxy, verificadas también con `-race`).

### Cómo simular la caída

```bash
curl -X POST http://localhost:9092/demo/crash
```

Responde HTTP 500 indicando el PID, espera 500 ms y termina con `os.Exit(1)`. Es la misma mecánica que `orders-api` y `orders-worker`; existe solo para la demostración de supervisión y nunca se invoca a sí misma.

> **Advertencia:** al igual que en `orders-api`, `/demo/crash` es una ruta académica. No debe habilitarse en un despliegue real.

### Cómo supervisarlo

`configs/example.json` registra `orders-web` como un proceso más:

```json
{
  "name": "orders-web",
  "command": "./bin/orders-web",
  "args": [],
  "restart": "on-failure",
  "workdir": ".",
  "max_retries": 5
}
```

Se usa `restart: "on-failure"` porque `/demo/crash` termina con código distinto de cero: una parada normal (SIGINT/SIGTERM) no debe reiniciarse, pero un fallo real sí. Desde el dashboard del scheduler en <http://localhost:8090> puede iniciarse, detenerse, reiniciarse y consultar el PID, los reintentos y los logs de `orders-web` igual que con cualquier otro job.

### Endpoints propios

| Método | Ruta | Resultado |
|---|---|---|
| GET | `/` | Dashboard |
| GET | `/health` | Salud de orders-web (`status`, `pid`, `time`, `uptime_seconds`, `orders_api_url`) |
| GET | `/assets/*` | CSS, JS e imágenes estáticas |
| POST | `/demo/crash` | Caída controlada de demostración |

### Rutas proxy

| Método | Ruta | Reenvía a orders-api |
|---|---|---|
| GET | `/proxy/health` | `GET /health` |
| GET | `/proxy/orders` | `GET /api/orders` |
| POST | `/proxy/orders` | `POST /api/orders` |
| GET | `/proxy/orders/{id}` | `GET /api/orders/{id}` |
| PATCH | `/proxy/orders/{id}/status` | `PATCH /api/orders/{id}/status` |
| DELETE | `/proxy/orders/{id}` | `DELETE /api/orders/{id}` |
| GET | `/proxy/orders/stats` | `GET /api/orders/stats` |

No existe ningún proxy genérico: cada ruta está registrada explícitamente y el destino nunca se toma de la solicitud del navegador. Los IDs se validan como enteros positivos y los `POST`/`PATCH` exigen `Content-Type: application/json` antes de decodificar el cuerpo (con límite de tamaño y `DisallowUnknownFields`).

### Decisiones técnicas

- El navegador solo habla con `localhost:9092`; el proxy evita CORS y centraliza timeouts y traducción de errores en un único punto (`writeUpstreamError`).
- El cliente HTTP distingue tres clases de fallo: `APIError` (orders-api respondió con un JSON de error reconocible y se reenvía tal cual, con su código y mensaje originales), y los sentinel `ErrUnavailable`/`ErrTimeout`/`ErrBadGateway` para cuando la respuesta no pudo obtenerse o interpretarse en absoluto — se traducen a 503, 504 y 502 respectivamente.
- Se usa el `http.ServeMux` de Go 1.22+ con patrones `MÉTODO /ruta` (incluidos parámetros `{id}`): un método no registrado en una ruta existente devuelve 405 automáticamente, sin lógica adicional.
- El HTML dinámico en JavaScript se construye con `createElement`/`textContent`, nunca con `innerHTML` sobre datos de pedidos, para no arriesgar inyección con nombres de cliente o producto que contengan caracteres especiales.
- Confirmaciones destructivas (eliminar, cancelar un pedido completado) usan el `<dialog>` nativo de `confirm-modal` en vez de `confirm()`/`alert()`, lo que también da cierre con `Escape` sin código adicional.

### Explicación del polling

El dashboard consulta `/proxy/health` y, en paralelo, `/proxy/orders` + `/proxy/orders/stats` cada 5 segundos (configurable a futuro vía `DashboardRefresh` en el config del servidor). No se dispara un nuevo ciclo si el anterior sigue en curso, y cada solicitud del navegador tiene su propio timeout con `AbortController`. Con `document.visibilityState === "hidden"` los intervalos se detienen; al volver a `"visible"` se fuerza una actualización inmediata y se reinician. Se eligió polling y no WebSocket por la misma razón que el scheduler: el volumen de datos es pequeño, tolera reconexiones sin estado adicional y mantiene tanto el servidor como la exposición oral más simples.

### Manejo cuando orders-api está caída

`orders-web` sigue sirviendo `/` y su propio `/health` aunque `orders-api` esté inalcanzable. El indicador de conexión pasa a "desconectado", aparece un aviso visible, y el formulario de registro y las acciones sobre pedidos se deshabilitan hasta que `/proxy/health` vuelva a responder 200 (por polling o pulsando "Verificar conexión"). Ningún handler del proxy hace panic ante un `orders-api` caído: el cliente HTTP siempre devuelve un error tipado y el handler lo traduce a un JSON `{"error": "...", "message": "..."}` con el código HTTP correspondiente (503 sin conexión, 504 timeout, 502 respuesta inesperada).

### Capturas sugeridas para la exposición

1. Dashboard con `orders-api` conectada: indicador verde, estadísticas y tabla con pedidos.
2. Formulario de registro con el total estimado calculándose en vivo.
3. Modal de detalle con el selector de estado y el botón de eliminar.
4. Dashboard justo después de detener `orders-api`: indicador rojo, aviso "API desconectada" y acciones deshabilitadas.
5. Dashboard tras reiniciar `orders-api`: reconexión automática con datos actualizados.
6. Terminal con `curl -X POST http://localhost:9092/demo/crash` y el log de `orders-web` mostrando el apagado, seguido del dashboard del scheduler reiniciando el proceso.
