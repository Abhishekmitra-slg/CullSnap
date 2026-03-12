import { useState, useEffect } from 'react';
import { model } from '../../wailsjs/go/models';

interface PhotoEXIF {
    camera: string;
    lens: string;
    iso: string;
    aperture: string;
    shutter: string;
    dateTaken: string;
}

interface ViewerProps {
    photo: model.Photo | null;
}

export function Viewer({ photo }: ViewerProps) {
    const [exif, setExif] = useState<PhotoEXIF | null>(null);

    useEffect(() => {
        if (!photo) {
            setExif(null);
            return;
        }

        // Try to load EXIF from backend
        const loadExif = async () => {
            try {
                // GetPhotoEXIF is a Wails binding we'll add
                const { GetPhotoEXIF } = await import('../../wailsjs/go/app/App');
                const data = await GetPhotoEXIF(photo.Path);
                if (data) {
                    setExif({
                        camera: data.camera || '—',
                        lens: data.lens || '—',
                        iso: data.iso || '—',
                        aperture: data.aperture || '—',
                        shutter: data.shutter || '—',
                        dateTaken: data.dateTaken || '—',
                    });
                }
            } catch {
                // EXIF not available, that's fine
                setExif(null);
            }
        };
        loadExif();
    }, [photo?.Path]);

    if (!photo) {
        return (
            <div className="viewer-panel">
                <div className="viewer-image-container">
                    <div style={{ color: 'var(--text-muted)', textAlign: 'center' }}>
                        <svg width="56" height="56" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1" strokeLinecap="round" strokeLinejoin="round" style={{ opacity: 0.3 }}>
                            <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
                            <circle cx="8.5" cy="8.5" r="1.5" />
                            <polyline points="21 15 16 10 5 21" />
                        </svg>
                        <p style={{ marginTop: 12, fontSize: '0.875rem' }}>Select a photo to view</p>
                    </div>
                </div>
            </div>
        );
    }

    const filename = photo.Path.split('/').pop();
    const kbSize = (photo.Size / 1024).toFixed(1);
    const mbSize = (photo.Size / 1024 / 1024).toFixed(1);
    const sizeText = photo.Size > 1024 * 1024 ? `${mbSize} MB` : `${kbSize} KB`;

    return (
        <div className="viewer-panel">
            {/* Main image */}
            <div className="viewer-image-container">
                <img
                    src={photo.Path}
                    alt={filename}
                    className="viewer-image"
                />
            </div>

            {/* Filename overlay */}
            <div className="viewer-info-overlay glass-panel">
                <div style={{ fontWeight: 600, color: 'white', wordBreak: 'break-all', fontSize: '0.8125rem' }}>
                    {filename}
                </div>
                <div style={{ color: 'var(--text-secondary)', fontSize: '0.75rem' }}>{sizeText}</div>
            </div>

            {/* EXIF Panel */}
            {exif && (
                <div className="exif-panel">
                    <ExifItem label="Camera" value={exif.camera} />
                    <ExifItem label="Lens" value={exif.lens} />
                    <ExifItem label="ISO" value={exif.iso} />
                    <ExifItem label="Aperture" value={exif.aperture} />
                    <ExifItem label="Shutter" value={exif.shutter} />
                    <ExifItem label="Date" value={exif.dateTaken} />
                </div>
            )}
        </div>
    );
}

function ExifItem({ label, value }: { label: string; value: string }) {
    return (
        <div className="exif-item">
            <span className="exif-label">{label}</span>
            <span className="exif-value">{value}</span>
        </div>
    );
}
