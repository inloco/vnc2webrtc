class Vnc2webrtc < Formula
  desc "WebRTC Streamer VNC Client"
  homepage "https://github.com/inloco/vnc2webrtc"

  bottle :unneeded
  head "https://github.com/inloco/vnc2webrtc.git", branch: "master"

  depends_on "go" => :build
  depends_on "libvncserver"
  depends_on "libvpx"

  def install
    system "go", "build"
    bin.install name
  end
end
