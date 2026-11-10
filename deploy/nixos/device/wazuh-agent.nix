{ lib, stdenv, fetchurl, dpkg, autoPatchelfHook, zlib, libgcc, elfutils }:

# Wazuh agent, repackaged from the upstream Debian binary (no source build:
# Wazuh ships a large C tree; the official .deb is the supported artifact).
# autoPatchelf rewrites the interpreter/rpath so the /var/ossec binaries run
# on NixOS. Runtime state lives under a StateDirectory, not the store.
stdenv.mkDerivation rec {
  pname = "wazuh-agent";
  version = "4.12.0";

  src = fetchurl {
    url = "https://packages.wazuh.com/4.x/apt/pool/main/w/wazuh-agent/wazuh-agent_${version}-1_amd64.deb";
    hash = "sha256-QdTuMIqsl8nQhz6KRVuUTaGEnWEc43hKtx1aOYFhRoU=";
  };

  nativeBuildInputs = [ dpkg autoPatchelfHook ];
  buildInputs = [ zlib libgcc elfutils stdenv.cc.cc.lib ];

  unpackPhase = "dpkg-deb -x $src .";

  installPhase = ''
    runHook preInstall
    mkdir -p $out
    cp -r var/ossec $out/ossec
    # The agent hard-codes /var/ossec at runtime; the systemd unit binds the
    # store copy over a writable state dir. Keep bin/ on PATH.
    mkdir -p $out/bin
    for b in $out/ossec/bin/*; do ln -s "$b" "$out/bin/$(basename "$b")"; done
    runHook postInstall
  '';

  # The bundled libs sit in ossec/lib; let autoPatchelf find them.
  appendRunpaths = [ "${placeholder "out"}/ossec/lib" ];

  meta = with lib; {
    description = "Wazuh endpoint security agent (repackaged from upstream .deb)";
    homepage = "https://wazuh.com";
    license = licenses.gpl2Only;
    platforms = [ "x86_64-linux" ];
  };
}
