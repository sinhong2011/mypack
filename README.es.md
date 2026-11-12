<div align="center">

# Mypack

**Cualquier sitio web. Cualquier escritorio. Un hermoso constructor nativo.**

Convierte un sitio web, o un proyecto HTML local, en una aplicación de escritorio real e independiente en unos clics. No hace falta un proyecto de Go, Rust, Node ni Xcode.

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![CI](https://github.com/sinhong2011/mypack/actions/workflows/ci.yml/badge.svg)](https://github.com/sinhong2011/mypack/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27.3%2B-00ADD8?logo=go&logoColor=white)](go.mod)
![Platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)
[![PRs welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

[English](README.md) · [繁體中文](README.zh-TW.md) · [简体中文](README.zh-CN.md) · [粵語](README.yue.md) · **Español** · [Français](README.fr.md) · [日本語](README.ja.md)

[Características](#características) · [Cómo empezar](#cómo-empezar) · [CLI y agentes](#cli-y-agentes-de-ia) · [Contribuir](#contribuir) · [Inspiración](#inspiración-y-agradecimientos)

![Biblioteca de Mypack](docs/screenshots/real-library.png)

</div>

```
Pega una URL  →  confirma el nombre y el icono  →  Compilar  →  Instalar y abrir
```

Mypack es gratuito y de código abierto bajo la [Apache License 2.0](LICENSE).
Todas las funciones están incluidas: sin límite de apps, sin claves de licencia, sin cuenta.

## ¿Por qué Mypack?

Gran parte de nuestro trabajo vive en aplicaciones web, pero una pestaña del navegador es un mal hogar para ellas.
Las pestañas se pierden entre otras pestañas, todos los sitios comparten un mismo juego de cookies y ninguno tiene su propio icono en el Dock, notificaciones o ventana.

Mypack le da a cada sitio una app propia de verdad, y después sigue cuidándola:

- **Sin cadena de herramientas en tu máquina.** Mypack no compila nada. Usa un runtime prediseñado pequeño y escribe en él la configuración de tu app, así que una compilación tarda segundos.
- **Apps que se sostienen por sí solas.** Cada app tiene su propio nombre, icono e inicio de sesión independiente, usa el WebView del sistema y sigue funcionando si desinstalas Mypack.
- **Cuidada, no solo creada.** Actualiza una app en un clic sin perder tus inicios de sesión (si una actualización falla, vuelve la versión anterior), repárala o muévela a otro equipo.

## Características

**Crear apps**

- 🌐 **Desde cualquier sitio web**: Mypack obtiene el nombre y el mejor icono. Puedes cambiar ambos, o diseñar un icono en el Estudio de iconos.
- 📁 **Desde proyectos locales**: un archivo HTML, una carpeta con `index.html`, o una exportación de Vite, CRA, Astro, Nuxt, SvelteKit o Next. Mypack ejecuta primero la compilación del proyecto, pero solo después de que la apruebes.
- 🧭 **Descubrir**: decenas de sitios populares, un clic para empezar. La lista es un [archivo JSON](catalog/discover.json) sencillo al que cualquiera puede añadir entradas.
- 🗂️ **Apps de grupo**: varios sitios en una misma ventana, con una barra lateral, una sesión independiente por sitio y vista dividida.
- 📥 **Configuraciones de Pake**: trae tu `pake.json` de [Pake](https://github.com/tw93/Pake).

**Dentro de las apps que creas**

- 🔔 Notificaciones web y una insignia en el Dock para los recuentos de no leídos
- 🔗 Los enlaces entre tus apps de Mypack se abren en la app correcta
- 🪟 Modos de ventana: ventana, compacta, barra lateral, o en la barra de menús
- 📑 Pestañas con páginas fijadas
- 🛡️ Bloqueo de contenido, CSS y JavaScript personalizados, ajustes de proxy y user-agent
- 🔑 Ventanas emergentes de inicio de sesión (Google, Microsoft y otras) que simplemente funcionan

**Gestionarlas**

- 📚 Una Biblioteca de todo lo que has creado, con Instalar y abrir, Actualizar, Reparar y Desinstalar
- 🔄 Sincronización de la Biblioteca entre equipos a través de una carpeta compartida como iCloud Drive, o exportación e importación de archivos `.mypack`
- 🧩 Una extensión de navegador opcional que convierte la pestaña en la que estás en una app
- 🌏 Siete idiomas: English, 繁體中文, 简体中文, 粵語, Español, Français, 日本語

**Para desarrolladores y agentes de IA**

- ⌨️ Una CLI sin interfaz, con salida `--json` y códigos de salida estables
- 🤖 Un servidor [MCP](https://modelcontextprotocol.io) integrado, para que Claude Code, Codex y otros agentes puedan crear y compilar apps por ti. Mypack no incluye chat ni un LLM propio.

## Cómo funciona

```
               ┌───────────── Mypack (el constructor) ─────────────┐
  sitio web /  │  nombre e icono  →  mypack.json  →  ensamblar app │      tu app
  carpeta  ──▶ │                                       ▲          │ ──▶  .app / .exe /
               └───────────────────────────────────────┼──────────┘      carpeta Linux
                                                       │
                          runtime prediseñado, descargado una vez desde un
                          registro firmado (Ed25519), verificado y puesto en caché
```

El runtime es un binario pequeño por plataforma que no sabe nada de ningún sitio web en particular. Mypack lo copia, añade el nombre, el icono y el `mypack.json` de tu app, y en macOS firma el resultado. Los detalles están en
[ARCHITECTURE.md](ARCHITECTURE.md).

| | Mypack (el constructor) | Apps que crea |
|---|---|---|
| **macOS** (Apple silicon e Intel) | ✅ Plataforma principal, empaquetado como `.app` / `.dmg` | ✅ WKWebView |
| **Linux** (x64 y arm64) | 🧪 Compilar desde el código fuente | ✅ WebKitGTK, probado en Ubuntu 24.04 |
| **Windows** (x64 y arm64) | 🧪 Compilar desde el código fuente | 🧪 WebView2, menos probado |

## Cómo empezar

### Instalar

**macOS** con [Homebrew](https://brew.sh):

```sh
brew install --cask sinhong2011/tap/mypack
```

**Windows** con winget, [Scoop](https://scoop.sh) o [Chocolatey](https://chocolatey.org):

```powershell
winget install Mypack.Mypack
```

```powershell
scoop bucket add mypack https://github.com/sinhong2011/scoop-bucket
scoop install mypack/mypack
```

```powershell
choco install mypack
```

**Debian y Ubuntu** (APT):

```sh
curl -fsSL https://sinhong2011.github.io/mypack/mypack.gpg | sudo tee /usr/share/keyrings/mypack.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/mypack.gpg] https://sinhong2011.github.io/mypack/apt stable main" | sudo tee /etc/apt/sources.list.d/mypack.list
sudo apt update && sudo apt install mypack
```

**Fedora, RHEL y openSUSE** (DNF, YUM, Zypper):

```sh
sudo curl -fsSL -o /etc/yum.repos.d/mypack.repo https://sinhong2011.github.io/mypack/rpm/mypack.repo
sudo dnf install mypack        # sudo zypper install mypack
```

**Arch Linux** desde [AUR](https://aur.archlinux.org/packages/mypack-bin):

```sh
yay -S mypack-bin
```

Cada [Release](https://github.com/sinhong2011/mypack/releases/latest) también incluye los instaladores para descargar directamente: un `.dmg` para macOS, programas de instalación para Windows y paquetes `.deb` y `.rpm` para Linux. Con un gestor de paquetes, el comando `mypack` queda en tu PATH y se actualiza con el comando de actualización de ese gestor.

> [!NOTE]
> Las versiones aún no están firmadas. En macOS, si Gatekeeper bloquea el primer inicio, haz clic derecho en Mypack.app y elige **Abrir**. En Windows, SmartScreen puede pedir confirmación (**Más información › Ejecutar de todas formas**).

### Compilar desde el código fuente

Necesitas Go 1.27.3 o una versión posterior. Con `GOTOOLCHAIN=auto` (el valor predeterminado), Go descarga por ti la versión correcta.

```sh
git clone https://github.com/sinhong2011/mypack.git
cd mypack
make runtimes   # build the app runtimes and sign them with a local dev key
make app        # build dist/Mypack.app (macOS)
open dist/Mypack.app
```

`make dmg` también crea una imagen de disco para arrastrar a Applications. Para explorarlo sin compilar nada, `make run-fake` inicia la app con datos de demostración.

### Crea tu primera app

1. Haz clic en **Nueva app** (⌘N) y pega una dirección, por ejemplo `github.com`.
2. Revisa el nombre y el icono.
3. Haz clic en **Compilar** y luego en **Instalar y abrir**.

También puedes soltar un archivo HTML o una carpeta sobre la ventana, o elegir un sitio en **Descubrir**.

## CLI y agentes de IA

El binario de la app también es la CLI:

```sh
alias mypack=/Applications/Mypack.app/Contents/MacOS/Mypack   # not needed after a package-manager install

mypack create github.com --json          # a draft project; nothing is built yet
mypack build <project-id> --yes --json   # build it (--yes allows the runtime download)
mypack install <artifact-id> --open
mypack apps list
```

Con `--json`, cada comando imprime un objeto JSON. Los códigos de salida son estables: `0` correcto, `1` error, `2` uso, `3` entrada no válida, `4` runtime o red, `5` la compilación falló, `6` se requiere aprobación, `130` cancelado. Ejecuta `mypack help` para ver todos los comandos.

**Servidor MCP.** Conecta un agente con:

```sh
claude mcp add mypack -- mypack mcp serve
```

**Ajustes › Conexiones de agentes** en la app tiene configuraciones listas para copiar para Claude Code, Codex y otros clientes MCP. Los agentes pueden listar, crear y compilar proyectos, pero no pueden instalar apps, y el script de compilación de un proyecto solo se ejecuta después de que lo apruebes.

**Extensión del navegador (opcional).** Consulta [docs/companion.md](docs/companion.md).

## Estructura del proyecto

```
cmd/          puntos de entrada: la app, la CLI, el host de native-messaging del navegador
runtime/      el runtime dentro de cada app que Mypack crea
internal/     constructor, instalador, servicio, almacenamiento, CLI, servidor MCP y la UI de la app
pkg/config/   el formato mypack.json (y la importación de Pake)
catalog/      el catálogo de Descubrir, discover.json
apps/         la extensión del navegador (WXT + SolidJS)
```

## Contribuir

Se agradecen las contribuciones de cualquier tamaño:

- **Añadir un sitio a Descubrir**: edita [`catalog/discover.json`](catalog/discover.json).
  Llega a todos los usuarios en un día, sin necesidad de una nueva versión.
- **Informar de un sitio que se comporta mal** con el formulario de incidencias *Website compatibility*.
- **Probar en Linux y Windows**: es donde ahora más se necesita ayuda.
- **Traducir, corregir errores, mejorar la documentación.**

Empieza por [CONTRIBUTING.md](CONTRIBUTING.md). Informa de los problemas de seguridad en privado, como se describe en [SECURITY.md](SECURITY.md), y sigue el
[Code of Conduct](CODE_OF_CONDUCT.md).

## Inspiración y agradecimientos

Mypack se apoya en los hombros de dos proyectos:

- **[Pake](https://github.com/tw93/Pake)** de [tw93](https://github.com/tw93)
  mostró lo bien que puede sentirse un sitio web como una app de escritorio pequeña y centrada, y cuánta gente quiere eso. Mypack toma la misma idea por otro camino: un runtime prediseñado en lugar de una cadena de herramientas de compilación, y una app nativa para crear y cuidar tus apps a lo largo del tiempo. Lee configuraciones de Pake, así que puedes traer las tuyas.
- **[MyGo](https://github.com/egoist/mygo)** de [egoist](https://github.com/egoist)
  es el kit de UI nativa escrito en Go puro detrás de la interfaz de Mypack y de las apps que crea: sin Electron, sin front end web, sin cgo.

Gracias también a [Lucide](https://lucide.dev) por los iconos y a
[modernc.org/sqlite](https://gitlab.com/cznic/sqlite). Todos los avisos de terceros
están en [NOTICE](NOTICE).

## Licencia

Mypack se licencia bajo la [Apache License 2.0](LICENSE).

El nombre «Mypack» y su logotipo son marcas comerciales y no están cubiertos por la licencia del código. Si distribuyes una compilación modificada, dale un nombre y un icono distintos; consulta [TRADEMARKS.md](TRADEMARKS.md). Los forks que distribuyan compilaciones
deben alojar su propio registro de runtimes y firmarlo con su propia clave.
