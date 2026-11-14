<div align="center">

# MyPack

**N’importe quel site. N’importe quel bureau. Un bel outil de création natif.**

Transformez un site web, ou un projet HTML local, en une véritable application de bureau autonome en quelques clics. Aucun projet Go, Rust, Node ou Xcode n’est requis.

[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![CI](https://github.com/sinhong2011/mypack/actions/workflows/ci.yml/badge.svg)](https://github.com/sinhong2011/mypack/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27.3%2B-00ADD8?logo=go&logoColor=white)](go.mod)
![Platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey)
[![PRs welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Listed on Awesome MyGo](https://awesomemygo.dev/badge.svg)](https://awesomemygo.dev/apps/mypack)

[English](README.md) · [繁體中文](README.zh-TW.md) · [简体中文](README.zh-CN.md) · [粵語](README.yue.md) · [Español](README.es.md) · **Français** · [日本語](README.ja.md)

[Fonctionnalités](#fonctionnalités) · [Pour commencer](#pour-commencer) · [CLI et agents](#cli-et-agents-ia) · [Contribuer](#contribuer) · [Inspiration](#inspiration-et-remerciements)

![Bibliothèque MyPack](docs/screenshots/real-library.png)

</div>

```
Collez une URL  →  confirmez le nom et l’icône  →  Créer  →  Installer et ouvrir
```

MyPack est libre et open source sous la [Apache License 2.0](LICENSE).
Toutes les fonctions sont incluses : pas de limite d’apps, pas de clés de licence, pas de compte.

## Pourquoi MyPack?

Une grande partie de notre travail vit dans des applications web, mais un onglet de navigateur est un mauvais foyer pour elles.
Les onglets se perdent parmi d’autres onglets, chaque site partage un même jeu de cookies, et aucun n’a sa propre icône dans le Dock, ses notifications ou sa fenêtre.

MyPack donne à chaque site une véritable application à soi, puis continue de s’en occuper :

- **Aucune chaîne d’outils sur votre machine.** MyPack ne compile rien. Il utilise un petit runtime préconstruit et y écrit les réglages de votre application, si bien qu’une création ne prend que quelques secondes.
- **Des applications autonomes.** Chaque application a son propre nom, sa propre icône et une session de connexion séparée, utilise le WebView du système, et continue de fonctionner si vous désinstallez MyPack.
- **Entretenues, pas seulement créées.** Mettez une application à jour en un clic sans perdre vos connexions (si une mise à jour échoue, la version précédente revient), réparez-la, ou déplacez-la vers un autre ordinateur.

## Fonctionnalités

**Créer des applications**

- 🌐 **Depuis n’importe quel site web** : MyPack récupère le nom et la meilleure icône. Vous pouvez modifier les deux, ou concevoir une icône dans le Studio d’icônes.
- 📁 **Depuis des projets locaux** : un fichier HTML, un dossier contenant `index.html`, ou un export Vite, CRA, Astro, Nuxt, SvelteKit ou Next. MyPack exécute d’abord la compilation du projet, mais seulement après votre approbation.
- 🧭 **Découvrir** : des dizaines de sites populaires, un clic pour commencer. La liste est un simple [fichier JSON](catalog/discover.json) auquel chacun peut contribuer.
- 🗂️ **Applications groupées** : plusieurs sites dans une même fenêtre, avec un rail latéral, une session distincte par site et une vue partagée.
- 📥 **Configurations Pake** : apportez votre `pake.json` de [Pake](https://github.com/tw93/Pake).

**Dans les applications que vous créez**

- 🔔 Notifications web et un badge dans le Dock pour les compteurs de non-lus
- 🎵 Touches multimédia et « À l’écoute » pour les sites de musique et de vidéo (macOS et Linux ; Windows les a déjà)
- 🔗 Les liens entre vos applications MyPack s’ouvrent dans la bonne application
- 🪟 Modes de fenêtre : fenêtre, compacte, barre latérale, ou dans la barre des menus
- 📑 Onglets avec pages épinglées
- 🛡️ Blocage de contenu, CSS et JavaScript personnalisés, réglages de proxy et de user-agent
- 🔑 Fenêtres contextuelles de connexion (Google, Microsoft et d’autres) qui fonctionnent tout simplement

**Les gérer**

- 📚 Une Bibliothèque de tout ce que vous avez créé, avec Installer et ouvrir, Mettre à jour, Réparer et Désinstaller
- 🔄 Synchronisation de la Bibliothèque entre ordinateurs via un dossier partagé tel qu’iCloud Drive, ou export et import de fichiers `.mypack`
- 🧩 Une extension de navigateur facultative qui transforme l’onglet en cours en application
- 🌏 Sept langues : English, 繁體中文, 简体中文, 粵語, Español, Français, 日本語

**Pour les développeurs et les agents IA**

- ⌨️ Une CLI sans interface, avec une sortie `--json` et des codes de sortie stables
- 🤖 Un serveur [MCP](https://modelcontextprotocol.io) intégré, pour que Claude Code, Codex et d’autres agents puissent créer et construire des applications pour vous. MyPack n’a pas de chat ni de LLM intégré.

## Fonctionnement

```
               ┌───────────── MyPack (l’outil de création) ─────────────┐
  site web /   │  nom et icône  →  mypack.json  →  assembler l’app     │      votre app
  dossier  ──▶ │                                       ▲               │ ──▶  .app / .exe /
               └───────────────────────────────────────┼───────────────┘      dossier Linux
                                                       │
                          runtime préconstruit, téléchargé une fois depuis un
                          registre signé (Ed25519), vérifié et mis en cache
```

Le runtime est un petit binaire par plateforme qui ne sait rien d’un site web en particulier. MyPack le copie, ajoute le nom, l’icône et le `mypack.json` de votre application, et sur macOS signe le résultat. Les détails se trouvent dans
[ARCHITECTURE.md](ARCHITECTURE.md).

| | MyPack (l’outil de création) | Applications qu’il crée |
|---|---|---|
| **macOS** (Apple silicon et Intel) | ✅ Plateforme principale, empaqueté en `.app` / `.dmg` | ✅ WKWebView |
| **Linux** (x64 et arm64) | 🧪 Compiler depuis les sources | ✅ WebKitGTK, testé sur Ubuntu 24.04 |
| **Windows** (x64 et arm64) | 🧪 Compiler depuis les sources | 🧪 WebView2, moins testé |

## Pour commencer

### Installer

<div align="center">

[![Télécharger pour macOS](https://img.shields.io/badge/T%C3%A9l%C3%A9charger_pour-macOS-555555?style=for-the-badge&logo=apple&logoColor=white)](https://github.com/sinhong2011/mypack/releases/latest)
[![Télécharger pour Windows](https://img.shields.io/badge/T%C3%A9l%C3%A9charger_pour-Windows-0078D4?style=for-the-badge&logo=windows&logoColor=white)](https://github.com/sinhong2011/mypack/releases/latest)
[![Télécharger pour Linux](https://img.shields.io/badge/T%C3%A9l%C3%A9charger_pour-Linux-FCC624?style=for-the-badge&logo=linux&logoColor=black)](https://github.com/sinhong2011/mypack/releases/latest)

</div>

**macOS** avec [Homebrew](https://brew.sh) :

```sh
brew install --cask sinhong2011/tap/mypack
```

**Windows** avec winget, [Scoop](https://scoop.sh) ou [Chocolatey](https://chocolatey.org) :

```powershell
winget install MyPack.MyPack
```

```powershell
scoop bucket add mypack https://github.com/sinhong2011/scoop-bucket
scoop install mypack/mypack
```

```powershell
choco install mypack
```

**Debian et Ubuntu** (APT) :

```sh
curl -fsSL https://sinhong2011.github.io/mypack/mypack.gpg | sudo tee /usr/share/keyrings/mypack.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/mypack.gpg] https://sinhong2011.github.io/mypack/apt stable main" | sudo tee /etc/apt/sources.list.d/mypack.list
sudo apt update && sudo apt install mypack
```

**Fedora, RHEL et openSUSE** (DNF, YUM, Zypper) :

```sh
sudo curl -fsSL -o /etc/yum.repos.d/mypack.repo https://sinhong2011.github.io/mypack/rpm/mypack.repo
sudo dnf install mypack        # sudo zypper install mypack
```

**Arch Linux** depuis l’[AUR](https://aur.archlinux.org/packages/mypack-bin) :

```sh
yay -S mypack-bin
```

Chaque [Release](https://github.com/sinhong2011/mypack/releases/latest) propose aussi les installeurs à télécharger directement : un `.dmg` pour macOS, des programmes d’installation pour Windows et des paquets `.deb` et `.rpm` pour Linux. Avec un gestionnaire de paquets, la commande `mypack` est dans votre PATH et se met à jour avec la commande de mise à jour de ce gestionnaire.

> [!NOTE]
> Les versions ne sont pas encore signées. Sur macOS, si Gatekeeper bloque le premier lancement, faites un clic droit sur MyPack.app puis **Ouvrir**. Sur Windows, SmartScreen peut demander une confirmation (**Informations complémentaires › Exécuter quand même**).

### Compiler depuis les sources

Vous avez besoin de Go 1.27.3 ou plus récent. Avec `GOTOOLCHAIN=auto` (la valeur par défaut), Go récupère pour vous la bonne version.

```sh
git clone https://github.com/sinhong2011/mypack.git
cd mypack
make runtimes   # build the app runtimes and sign them with a local dev key
make app        # build dist/MyPack.app (macOS)
open dist/MyPack.app
```

`make dmg` produit aussi une image disque à glisser dans Applications. Pour explorer sans rien compiler, `make run-fake` lance l’application avec des données de démonstration.

### Créer votre première application

1. Cliquez sur **Nouvelle application** (⌘N) et collez une adresse, par exemple `github.com`.
2. Vérifiez le nom et l’icône.
3. Cliquez sur **Créer**, puis sur **Installer et ouvrir**.

Vous pouvez aussi déposer un fichier HTML ou un dossier sur la fenêtre, ou choisir un site dans **Découvrir**.

## CLI et agents IA

Le binaire de l’application est aussi la CLI :

```sh
alias mypack=/Applications/MyPack.app/Contents/MacOS/MyPack   # not needed after a package-manager install

mypack create github.com --json          # a draft project; nothing is built yet
mypack build <project-id> --yes --json   # build it (--yes allows the runtime download)
mypack install <artifact-id> --open
mypack apps list
```

Avec `--json`, chaque commande affiche un objet JSON. Les codes de sortie sont stables : `0` succès, `1` erreur, `2` utilisation, `3` entrée invalide, `4` runtime ou réseau, `5` échec de la création, `6` approbation requise, `130` annulé. Exécutez `mypack help` pour toutes les commandes.

**Serveur MCP.** Connectez un agent avec :

```sh
claude mcp add mypack -- mypack mcp serve
```

**Réglages › Connexions d’agents** dans l’application propose des configurations prêtes à copier pour Claude Code, Codex et d’autres clients MCP. Les agents peuvent lister, créer et construire des projets, mais ils ne peuvent pas installer d’applications, et le script de compilation d’un projet ne s’exécute qu’après votre approbation.

**Extension de navigateur (facultative).** Voir [docs/companion.md](docs/companion.md).

## Structure du projet

```
cmd/          points d’entrée : l’application, la CLI, l’hôte native-messaging du navigateur
runtime/      le runtime à l’intérieur de chaque application créée par MyPack
internal/     outil de création, installateur, service, stockage, CLI, serveur MCP et l’UI de l’application
pkg/config/   le format mypack.json (et l’import Pake)
catalog/      le catalogue Découvrir, discover.json
apps/         l’extension de navigateur (WXT + SolidJS)
```

## Contribuer

Les contributions de toute taille sont les bienvenues :

- **Ajouter un site à Découvrir** : modifiez [`catalog/discover.json`](catalog/discover.json).
  Cela parvient à tous les utilisateurs en une journée, sans attendre une nouvelle version.
- **Signaler un site qui se comporte mal** avec le formulaire d’issue *Website compatibility*.
- **Tester sur Linux et Windows** : c’est là que l’aide compte le plus pour le moment.
- **Traduire, corriger des bugs, améliorer la documentation.**

Commencez par [CONTRIBUTING.md](CONTRIBUTING.md). Signalez les problèmes de sécurité en privé, comme décrit dans [SECURITY.md](SECURITY.md), et respectez le
[Code of Conduct](CODE_OF_CONDUCT.md).

## Inspiration et remerciements

MyPack se tient sur les épaules de deux projets :

- **[Pake](https://github.com/tw93/Pake)** par [tw93](https://github.com/tw93)
  a montré à quel point un site web peut être agréable en tant qu’application de bureau petite et concentrée, et combien de personnes le souhaitent. MyPack reprend la même idée autrement : un runtime préconstruit à la place d’une chaîne d’outils de compilation, et une application native pour créer et accompagner vos applications dans le temps. Il lit les configurations Pake, vous pouvez donc apporter les vôtres.
- **[MyGo](https://github.com/egoist/mygo)** par [egoist](https://github.com/egoist)
  est la boîte à outils d’UI native en Go pur derrière l’interface de MyPack et les applications qu’il crée : pas d’Electron, pas de front end web, pas de cgo.

Merci aussi à [Lucide](https://lucide.dev) pour les icônes et à
[modernc.org/sqlite](https://gitlab.com/cznic/sqlite). Toutes les mentions tierces
se trouvent dans [NOTICE](NOTICE).

## Licence

MyPack est sous licence [Apache License 2.0](LICENSE).

Le nom « MyPack » et son logo sont des marques, et ne sont pas couverts par la licence du code. Si vous distribuez une version modifiée, donnez-lui un autre nom et une autre icône ; voir [TRADEMARKS.md](TRADEMARKS.md). Les forks qui distribuent des versions
doivent héberger leur propre registre de runtimes et le signer avec leur propre clé.
