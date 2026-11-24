# Until the first release this formula builds from the main branch:
#   brew tap matteocodogno/respawn https://github.com/matteocodogno/respawn
#   brew install --HEAD matteocodogno/respawn/respawn
# Each release (a v* tag) regenerates it with prebuilt binaries (scripts/formula.sh).
class Respawn < Formula
  desc "Comic-book slide decks as code, with live terminals and a follow-along audience"
  homepage "https://github.com/matteocodogno/respawn"
  license "Apache-2.0"
  head "https://github.com/matteocodogno/respawn.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.version=HEAD"), "./cmd/respawn"
  end

  def caveats
    <<~EOS
      The PDF export needs Chrome, Chromium, Brave or Edge.
      The audience copy over the internet (respawn serve --tunnel) needs cloudflared:
        brew install cloudflared
    EOS
  end

  test do
    system bin/"respawn", "version"
  end
end
