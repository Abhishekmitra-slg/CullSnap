cask "cullsnap" do
  version "1.0.20"
  sha256 "cbb475bb8cf79e5a518480c1c2072cff3ca0440c830749f8d252d0a4a2c40efd"

  url "https://github.com/Abhishekmitra-slg/CullSnap/releases/download/v#{version}/CullSnap-macos-universal.zip"
  name "CullSnap"
  desc "High-performance photo culling tool for photographers"
  homepage "https://github.com/Abhishekmitra-slg/CullSnap"

  app "CullSnap.app"

  zap trash: [
    "~/.cullsnap",
    "~/Library/Application Support/CullSnap",
    "~/Library/Caches/CullSnap",
    "~/Library/Preferences/com.abhishekmitra.cullsnap.plist",
  ]
end
