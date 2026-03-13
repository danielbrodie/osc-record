class OscRecord < Formula
  desc "OSC-triggered video capture for live production"
  homepage "https://github.com/brodiegraphics/osc-record"
  version "0.1.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/brodiegraphics/osc-record/releases/download/v0.1.0/osc-record_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER"
    else
      url "https://github.com/brodiegraphics/osc-record/releases/download/v0.1.0/osc-record_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  depends_on "ffmpeg"

  def install
    bin.install "osc-record"
  end

  def caveats
    <<~EOS
      For Blackmagic capture devices, decklink mode is strongly recommended.
      It auto-detects signal format (resolution, framerate, pixel format).

      To enable decklink mode, install ffmpeg with decklink support:
        brew tap homebrew-ffmpeg/ffmpeg
        brew install homebrew-ffmpeg/ffmpeg/ffmpeg --with-decklink

      You also need the Blackmagic Desktop Video drivers installed:
        https://www.blackmagicdesign.com/support

      Without decklink support, osc-record falls back to avfoundation,
      which requires manual framerate and pixel format configuration.
    EOS
  end

  test do
    assert_match "osc-record v", shell_output("#{bin}/osc-record version")
  end
end
