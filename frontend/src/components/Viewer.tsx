import { useState, useEffect, useRef } from 'react';
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
    onTrimChange?: (path: string, start: number, end: number) => void;
}

export function Viewer({ photo, onTrimChange }: ViewerProps) {
    const [exif, setExif] = useState<PhotoEXIF | null>(null);
    const videoRef = useRef<HTMLVideoElement>(null);

    useEffect(() => {
        if (!photo || photo.IsVideo) {
            setExif(null);
            return;
        }

        const loadExif = async () => {
            try {
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
                setExif(null);
            }
        };
        loadExif();
    }, [photo?.Path, photo?.IsVideo]);

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
    const mbSize = (photo.Size / 1024 / 1024).toFixed(1);

    const handleSetStart = () => {
        if (videoRef.current && onTrimChange) {
            const currentT = videoRef.current.currentTime;
            const targetEnd = photo.TrimEnd > 0 ? photo.TrimEnd : photo.Duration;
            if (currentT < targetEnd) {
                onTrimChange(photo.Path, currentT, targetEnd);
            }
        }
    };

    const handleSetEnd = () => {
        if (videoRef.current && onTrimChange) {
            const currentT = videoRef.current.currentTime;
            if (currentT > photo.TrimStart) {
                onTrimChange(photo.Path, photo.TrimStart, currentT);
            }
        }
    };

    const handleSliderChange = (e: React.ChangeEvent<HTMLInputElement>, isStart: boolean) => {
        if (!onTrimChange) return;
        const val = parseFloat(e.target.value);
        const currentStart = photo.TrimStart;
        const currentEnd = photo.TrimEnd > 0 ? photo.TrimEnd : photo.Duration;

        if (isStart) {
            if (val < currentEnd) {
                onTrimChange(photo.Path, val, currentEnd);
                if (videoRef.current) videoRef.current.currentTime = val;
            }
        } else {
            if (val > currentStart) {
                onTrimChange(photo.Path, currentStart, val);
                if (videoRef.current) videoRef.current.currentTime = val;
            }
        }
    };

    return (
        <div className="viewer-panel" style={{ position: 'relative' }}>
            {/* Main media */}
            <div className="viewer-image-container" style={{ position: 'relative' }}>
                {photo.IsVideo ? (
                    <video
                        ref={videoRef}
                        src={`http://localhost:34342/wails-media?path=${encodeURIComponent(photo.Path)}`}
                        className="viewer-image" // reuse CSS
                        controls
                        style={{ maxHeight: 'calc(100vh - 120px)' }}
                    />
                ) : (
                    <img
                        src={`http://localhost:34342/wails-media?path=${encodeURIComponent(photo.Path)}`}
                        alt={filename}
                        className="viewer-image"
                    />
                )}
            </div>

            {/* Video Trimmer UI - Docked at bottom when viewing video */}
            {photo.IsVideo && (
                <div className="video-trim-panel glass-panel" style={{ position: 'absolute', bottom: 20, left: '50%', transform: 'translateX(-50%)', padding: '12px 20px', borderRadius: 12, display: 'flex', flexDirection: 'column', gap: 12, width: '400px', maxWidth: '90%' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <span style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Trim Export</span>
                        <div style={{ display: 'flex', gap: 8 }}>
                            <button className="btn outline" style={{ padding: '4px 10px', fontSize: '0.75rem' }} onClick={handleSetStart}>
                                Set Start
                            </button>
                            <button className="btn outline" style={{ padding: '4px 10px', fontSize: '0.75rem' }} onClick={handleSetEnd}>
                                Set End
                            </button>
                        </div>
                    </div>
                    
                    {/* Dual slider trick using two range inputs overlayed */}
                    <div style={{ position: 'relative', height: 20, display: 'flex', alignItems: 'center', margin: '4px 0' }}>
                        {/* Background track */}
                        <div style={{ position: 'absolute', width: '100%', height: 4, background: 'var(--border-color)', borderRadius: 2 }} />
                        
                        {/* Selected range highlight */}
                        <div style={{ 
                            position: 'absolute', 
                            height: 4, 
                            background: 'var(--accent)', 
                            borderRadius: 2,
                            left: `${(photo.TrimStart / photo.Duration) * 100}%`,
                            width: `${((photo.TrimEnd > 0 ? photo.TrimEnd : photo.Duration) - photo.TrimStart) / photo.Duration * 100}%`
                        }} />

                        {/* Start Thumb */}
                        <input 
                            type="range" 
                            min={0} 
                            max={photo.Duration} 
                            step={0.1}
                            value={photo.TrimStart} 
                            onChange={(e) => handleSliderChange(e, true)}
                            style={{ position: 'absolute', width: '100%', pointerEvents: 'none', background: 'transparent' }} 
                            className="trim-slider"
                        />
                        
                        {/* End Thumb */}
                        <input 
                            type="range" 
                            min={0} 
                            max={photo.Duration} 
                            step={0.1}
                            value={photo.TrimEnd > 0 ? photo.TrimEnd : photo.Duration} 
                            onChange={(e) => handleSliderChange(e, false)}
                            style={{ position: 'absolute', width: '100%', pointerEvents: 'none', background: 'transparent' }} 
                            className="trim-slider end-slider"
                        />
                    </div>

                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.7rem', color: 'var(--text-muted)' }}>
                        <span>Start: {photo.TrimStart.toFixed(1)}s</span>
                        <span>End: {(photo.TrimEnd > 0 ? photo.TrimEnd : photo.Duration).toFixed(1)}s</span>
                    </div>
                </div>
            )}

            {/* Filename overlay — top right */}
            <div className="viewer-info-overlay glass-panel">
                <div style={{ fontWeight: 600, color: 'white', wordBreak: 'break-all', fontSize: '0.8125rem' }}>
                    {filename}
                </div>
                <div style={{ color: 'var(--text-secondary)', fontSize: '0.75rem' }}>{mbSize} MB</div>
            </div>

            {/* EXIF Panel — bottom-right frosted glass card */}
            {exif && !photo.IsVideo && (
                <div className="exif-panel">
                    <div className="exif-panel-title">EXIF metadata</div>
                    <div className="exif-grid">
                        <ExifRow label="Camera" value={exif.camera} />
                        <ExifRow label="Lens" value={exif.lens} />
                        <ExifRow label="ISO" value={exif.iso} />
                        <ExifRow label="Aperture" value={exif.aperture} />
                        <ExifRow label="Shutter" value={exif.shutter} />
                        <ExifRow label="Date Taken" value={exif.dateTaken} />
                    </div>
                </div>
            )}
        </div>
    );
}

function ExifRow({ label, value }: { label: string; value: string }) {
    return (
        <div className="exif-item">
            <span className="exif-label">{label}</span>
            <span className="exif-value">{value}</span>
        </div>
    );
}
