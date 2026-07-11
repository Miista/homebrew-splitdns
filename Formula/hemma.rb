# typed: false
# frozen_string_literal: true

# Transitional formula for the splitdns -> hemma rename: it installs the last
# splitdns release under the new name (plus a `splitdns` alias symlink). The
# next GoReleaser release overwrites this file with hemma-named artifacts.
class Hemma < Formula
  desc "Generate split-horizon DNS (Pi-hole/dnsmasq), Caddy site blocks, and auth-provider config from a declarative services.yaml"
  homepage "https://github.com/Miista/homebrew-splitdns"
  version "0.23.1"
  license "MIT"

  on_macos do
    if Hardware::CPU.intel?
      url "https://github.com/Miista/homebrew-splitdns/releases/download/v0.23.1/splitdns_0.23.1_darwin_amd64.tar.gz"
      sha256 "bdbb813b00ab0199ce2d5571e7a732abe5c396d87f22f49e2c98096371c9e96e"

      define_method(:install) do
        bin.install "splitdns" => "hemma"
        bin.install_symlink "hemma" => "splitdns"
        man1.install "man/splitdns.1.gz"
        bash_completion.install "completions/splitdns.bash" => "splitdns"
        zsh_completion.install "completions/_splitdns.zsh" => "_splitdns"
      end
    end
    if Hardware::CPU.arm?
      url "https://github.com/Miista/homebrew-splitdns/releases/download/v0.23.1/splitdns_0.23.1_darwin_arm64.tar.gz"
      sha256 "2d285efb8cc50e30e0a33b3cbe305122c6c90823d7832dbe9d3c3f9301cf2590"

      define_method(:install) do
        bin.install "splitdns" => "hemma"
        bin.install_symlink "hemma" => "splitdns"
        man1.install "man/splitdns.1.gz"
        bash_completion.install "completions/splitdns.bash" => "splitdns"
        zsh_completion.install "completions/_splitdns.zsh" => "_splitdns"
      end
    end
  end

  on_linux do
    if Hardware::CPU.intel? && Hardware::CPU.is_64_bit?
      url "https://github.com/Miista/homebrew-splitdns/releases/download/v0.23.1/splitdns_0.23.1_linux_amd64.tar.gz"
      sha256 "0a43dc785e07ce515530a851dae7a2c1804ce9a6fba959f336b8ff32f9e4fa92"
      define_method(:install) do
        bin.install "splitdns" => "hemma"
        bin.install_symlink "hemma" => "splitdns"
        man1.install "man/splitdns.1.gz"
        bash_completion.install "completions/splitdns.bash" => "splitdns"
        zsh_completion.install "completions/_splitdns.zsh" => "_splitdns"
      end
    end
    if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
      url "https://github.com/Miista/homebrew-splitdns/releases/download/v0.23.1/splitdns_0.23.1_linux_arm64.tar.gz"
      sha256 "6ffbd19779dbce043d9462a8ee7abcc6cf2c0027341e1eb0e64eebd6c2f749af"
      define_method(:install) do
        bin.install "splitdns" => "hemma"
        bin.install_symlink "hemma" => "splitdns"
        man1.install "man/splitdns.1.gz"
        bash_completion.install "completions/splitdns.bash" => "splitdns"
        zsh_completion.install "completions/_splitdns.zsh" => "_splitdns"
      end
    end
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/hemma version")
  end
end
