/** Site-wide facts in one place. Every link and command here comes from the repo's README. */
const repo = "https://github.com/sinhong2011/mypack";

export const SITE = {
  name: "MyPack",
  repo,
  releases: `${repo}/releases/latest`,
  api: "https://api.github.com/repos/sinhong2011/mypack",
  license: `${repo}/blob/main/LICENSE`,
  trademarks: `${repo}/blob/main/TRADEMARKS.md`,
  contributing: `${repo}/blob/main/CONTRIBUTING.md`,
  security: `${repo}/blob/main/SECURITY.md`,
  issues: `${repo}/issues`,
  discover: `${repo}/blob/main/catalog/discover.json`,
  architecture: `${repo}/blob/main/ARCHITECTURE.md`,
  pake: "https://github.com/tw93/Pake",
  mygo: "https://github.com/egoist/mygo",
};

/** Package-manager installs per platform, verbatim from README.md › Install. */
export const INSTALL = {
  mac: [{ label: "Homebrew", code: "brew install --cask sinhong2011/tap/mypack" }],
  win: [
    { label: "winget", code: "winget install MyPack.MyPack" },
    { label: "Scoop", code: "scoop bucket add mypack https://github.com/sinhong2011/scoop-bucket\nscoop install mypack/mypack" },
    { label: "Chocolatey", code: "choco install mypack" },
  ],
  linux: [
    {
      label: "apt",
      code: 'curl -fsSL https://sinhong2011.github.io/mypack/mypack.gpg | sudo tee /usr/share/keyrings/mypack.gpg >/dev/null\necho "deb [signed-by=/usr/share/keyrings/mypack.gpg] https://sinhong2011.github.io/mypack/apt stable main" | sudo tee /etc/apt/sources.list.d/mypack.list\nsudo apt update && sudo apt install mypack',
    },
    { label: "dnf", code: "sudo curl -fsSL -o /etc/yum.repos.d/mypack.repo https://sinhong2011.github.io/mypack/rpm/mypack.repo\nsudo dnf install mypack" },
    { label: "AUR", code: "yay -S mypack-bin" },
  ],
} as const;

export const asset = (path: string) => `${import.meta.env.BASE_URL.replace(/\/$/, "")}/${path.replace(/^\//, "")}`;
