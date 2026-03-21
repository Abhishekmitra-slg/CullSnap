#!/bin/bash
# Downloads real camera RAW samples for testing.
# Run once before testing: bash internal/raw/testdata/download.sh
set -e
DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

echo "Downloading RAW test samples..."

# CR2 - Canon
[ -f sample.cr2 ] || curl -L -o sample.cr2 "https://raw.pixls.us/getfile.php/2267/nice/Canon%20-%20Canon%20EOS%205D%20Mark%20IV%20-%20IMG_0004.CR2" || echo "CR2 download failed"

# NEF - Nikon
[ -f sample.nef ] || curl -L -o sample.nef "https://raw.pixls.us/getfile.php/3102/nice/Nikon%20-%20D850.NEF" || echo "NEF download failed"

# ARW - Sony
[ -f sample.arw ] || curl -L -o sample.arw "https://raw.pixls.us/getfile.php/2803/nice/Sony%20-%20ILCE-7RM3.ARW" || echo "ARW download failed"

# DNG - Leica
[ -f sample.dng ] || curl -L -o sample.dng "https://raw.pixls.us/getfile.php/3090/nice/Leica%20-%20Q2.DNG" || echo "DNG download failed"

# RAF - Fujifilm
[ -f sample.raf ] || curl -L -o sample.raf "https://raw.pixls.us/getfile.php/2793/nice/Fujifilm%20-%20X-T3.RAF" || echo "RAF download failed"

# RW2 - Panasonic
[ -f sample.rw2 ] || curl -L -o sample.rw2 "https://raw.pixls.us/getfile.php/1878/nice/Panasonic%20-%20DC-GH5.RW2" || echo "RW2 download failed"

# ORF - Olympus
[ -f sample.orf ] || curl -L -o sample.orf "https://raw.pixls.us/getfile.php/2122/nice/Olympus%20-%20E-M1MarkII.ORF" || echo "ORF download failed"

echo "Done!"
