import { useState, useEffect, useRef } from 'react';
import { Check, Eye, EyeOff } from 'lucide-react';
import { model } from '../../wailsjs/go/models';

interface PhotoEXIF {
    camera: string;
    lens: string;
    iso: string;
    aperture: string;
    shutter: string;
    dateTaken: string;
}

interface FaceDetection {
    bboxX: number;
    bboxY: number;
    bboxW: number;
    bboxH: number;
    eyeSharpness: number;
    eyesOpen: boolean;
    expression: number;
    confidence: number;
    clusterId?: number;
}

interface ViewerProps {
    photo: model.Photo | null;
    onTrimChange?: (path: string, start: number, end: number) => void;
    isSelected?: boolean;
    faceDetections?: FaceDetection[];
    aiPanelVisible?: boolean;
    onFaceClick?: (clusterId: number) => void;
}

// Thumbnail width used by ThumbCache for coordinate mapping
const THUMB_WIDTH = 300;

export function Viewer({ photo, onTrimChange, isSelected, faceDetections, aiPanelVisible, onFaceClick }: ViewerProps) {
    const [exif, setExif] = useState<PhotoEXIF | null>(null);
    const [showFaceOverlay, setShowFaceOverlay] = useState(true);
    const [hoveredFace, setHoveredFace] = useState<number | null>(null);
    const videoRef = useRef<HTMLVideoElement>(null);
    const imageRef = useRef<HTMLImageElement>(null);
    const [imageSize, setImageSize] = useState({ width: 0, height: 0 });

    useEffect(() => {
        if (!photo || photo.IsVideo) {
            setExif(null);
            return;
        }

        let cancelled = false;
        const loadExif = async () => {
            try {
                const { GetPhotoEXIF } = await import('../../wailsjs/go/app/App');
                const data = await GetPhotoEXIF(photo.Path);
                if (!cancelled && data) {
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
                if (!cancelled) setExif(null);
            }
        };
        loadExif();
        return () => { cancelled = true; };
    }, [photo?.Path, photo?.IsVideo]);

    // Track displayed image dimensions via ResizeObserver
    useEffect(() => {
        const el = imageRef.current;
        if (!el) return;

        const observer = new ResizeObserver(() => {
            if (imageRef.current) {
                setImageSize({
                    width: imageRef.current.offsetWidth,
                    height: imageRef.current.offsetHeight,
                });
            }
        });
        observer.observe(el);

        // Set initial size
        setImageSize({ width: el.offsetWidth, height: el.offsetHeight });

        return () => observer.disconnect();
    }, [photo?.Path]);

    // Reset image size when photo changes
    useEffect(() => {
        setImageSize({ width: 0, height: 0 });
    }, [photo?.Path]);

    // Compute media URL inline — no state needed, avoids extra render cycle
    const mediaUrl = photo && !photo.IsVideo
        ? `http://localhost:34342/wails-media?path=${encodeURIComponent(photo.Path)}`
        : '';

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

    const filename = photo.Path.split(/[/\\]/).pop();
    const mbSize = (photo.Size / 1024 / 1024).toFixed(1);

    // Get the effective duration: prefer photo.Duration, fall back to the <video> element's duration.
    const getEffectiveDuration = () => {
        if (photo.Duration > 0) return photo.Duration;
        if (videoRef.current && videoRef.current.duration && isFinite(videoRef.current.duration)) return videoRef.current.duration;
        return 0;
    };

    const handleSetStart = () => {
        if (videoRef.current && onTrimChange) {
            const currentT = videoRef.current.currentTime;
            const dur = getEffectiveDuration();
            const targetEnd = photo.TrimEnd > 0 ? photo.TrimEnd : dur;
            if (dur > 0 && currentT < targetEnd) {
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
        const dur = getEffectiveDuration();
        const currentEnd = photo.TrimEnd > 0 ? photo.TrimEnd : dur;

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

    // Compute face box color based on eyeSharpness quality score
    const faceBoxColor = (eyeSharpness: number): string => {
        if (eyeSharpness >= 80) return '#22c55e'; // green
        if (eyeSharpness >= 50) return '#eab308'; // yellow
        return '#ef4444'; // red
    };

    // Scale face bounding box coords from 300px thumbnail space to displayed image size
    const scaleFaceBox = (face: FaceDetection) => {
        if (imageSize.width === 0 || imageSize.height === 0) return null;

        // Bounding boxes are in 300px thumbnail coordinate space.
        // The thumbnail is 300px wide; height is proportional.
        // We only know the width (THUMB_WIDTH), so scale by width ratio and apply to both axes
        // (thumbnails are generated with fixed width, proportional height).
        const scaleX = imageSize.width / THUMB_WIDTH;
        // Height of thumbnail is proportional: thumbH = THUMB_WIDTH * (naturalH / naturalW)
        // For displayed image the same ratio applies, so we use scaleX for both axes.
        const scaleY = scaleX;

        return {
            left: face.bboxX * scaleX,
            top: face.bboxY * scaleY,
            width: face.bboxW * scaleX,
            height: face.bboxH * scaleY,
        };
    };

    const hasFaces = faceDetections && faceDetections.length > 0 && !photo.IsVideo;

    return (
        <div className="viewer-panel" style={{ position: 'relative' }}>
            <div className={`viewer-selected-bar ${isSelected ? 'visible' : ''}`}>
                <Check size={14} />
                Selected
            </div>
            {/* Main media */}
            <div className="viewer-image-container" style={{ position: 'relative' }}>
                {photo.isRAW && photo.rawFormat && (
                    <div className="viewer-raw-badge">
                        {photo.rawFormat}{photo.companionPath ? ' + JPG' : ''}
                    </div>
                )}
                {photo.IsVideo ? (
                    <video
                        key={photo.Path}
                        ref={videoRef}
                        src={`http://localhost:34342/wails-media?path=${encodeURIComponent(photo.Path)}`}
                        className="viewer-image" // reuse CSS
                        controls
                        style={{ maxHeight: 'calc(100vh - 120px)' }}
                    />
                ) : (
                    <div style={{ position: 'relative', display: 'inline-block' }}>
                        <img
                            ref={imageRef}
                            src={mediaUrl}
                            alt={filename}
                            className="viewer-image"
                            onLoad={() => {
                                if (imageRef.current) {
                                    setImageSize({
                                        width: imageRef.current.offsetWidth,
                                        height: imageRef.current.offsetHeight,
                                    });
                                }
                            }}
                        />
                        {/* Face bounding box overlay */}
                        {hasFaces && showFaceOverlay && imageSize.width > 0 && (
                            <div
                                style={{
                                    position: 'absolute',
                                    top: 0,
                                    left: 0,
                                    width: imageSize.width,
                                    height: imageSize.height,
                                    pointerEvents: 'none',
                                }}
                            >
                                {faceDetections!.map((face, idx) => {
                                    const scaled = scaleFaceBox(face);
                                    if (!scaled) return null;
                                    const color = faceBoxColor(face.eyeSharpness);
                                    const isHovered = hoveredFace === idx;
                                    return (
                                        <div
                                            key={idx}
                                            style={{
                                                position: 'absolute',
                                                left: scaled.left,
                                                top: scaled.top,
                                                width: scaled.width,
                                                height: scaled.height,
                                                border: `2px solid ${color}`,
                                                borderRadius: 3,
                                                boxShadow: isHovered ? `0 0 0 1px ${color}` : undefined,
                                                cursor: face.clusterId !== undefined ? 'pointer' : 'default',
                                                pointerEvents: 'all',
                                                transition: 'box-shadow 0.15s ease',
                                            }}
                                            onMouseEnter={() => setHoveredFace(idx)}
                                            onMouseLeave={() => setHoveredFace(null)}
                                            onClick={() => {
                                                if (face.clusterId !== undefined && onFaceClick) {
                                                    onFaceClick(face.clusterId);
                                                }
                                            }}
                                        >
                                            {/* Hover label */}
                                            {isHovered && (
                                                <div
                                                    style={{
                                                        position: 'absolute',
                                                        bottom: '100%',
                                                        left: 0,
                                                        marginBottom: 4,
                                                        background: 'rgba(0,0,0,0.75)',
                                                        color: '#fff',
                                                        fontSize: '0.7rem',
                                                        padding: '2px 6px',
                                                        borderRadius: 4,
                                                        whiteSpace: 'nowrap',
                                                        pointerEvents: 'none',
                                                    }}
                                                >
                                                    {face.eyeSharpness}% sharpness
                                                    {!face.eyesOpen && ' · eyes closed'}
                                                </div>
                                            )}
                                        </div>
                                    );
                                })}
                            </div>
                        )}
                    </div>
                )}
            </div>

            {/* Face overlay toggle — top-left of viewer, only shown when AI panel visible and faces present */}
            {hasFaces && aiPanelVisible && (
                <button
                    onClick={() => setShowFaceOverlay(v => !v)}
                    title={showFaceOverlay ? 'Hide face boxes' : 'Show face boxes'}
                    style={{
                        position: 'absolute',
                        top: 12,
                        left: 12,
                        background: 'rgba(0,0,0,0.55)',
                        border: '1px solid rgba(255,255,255,0.15)',
                        borderRadius: 6,
                        color: '#fff',
                        width: 32,
                        height: 32,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        cursor: 'pointer',
                        zIndex: 10,
                        transition: 'background 0.15s ease',
                    }}
                    onMouseEnter={e => (e.currentTarget.style.background = 'rgba(0,0,0,0.75)')}
                    onMouseLeave={e => (e.currentTarget.style.background = 'rgba(0,0,0,0.55)')}
                >
                    {showFaceOverlay ? <Eye size={16} /> : <EyeOff size={16} />}
                </button>
            )}

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
                    {(() => {
                        const dur = getEffectiveDuration();
                        const effectiveEnd = photo.TrimEnd > 0 ? photo.TrimEnd : dur;
                        return (
                            <>
                                <div style={{ position: 'relative', height: 24, display: 'flex', alignItems: 'center', margin: '4px 0' }}>
                                    {/* Background track */}
                                    <div style={{ position: 'absolute', width: '100%', height: 4, background: 'var(--border-color)', borderRadius: 2 }} />

                                    {/* Selected range highlight */}
                                    {dur > 0 && (
                                        <div style={{
                                            position: 'absolute',
                                            height: 4,
                                            background: 'var(--accent)',
                                            borderRadius: 2,
                                            left: `${(photo.TrimStart / dur) * 100}%`,
                                            width: `${(effectiveEnd - photo.TrimStart) / dur * 100}%`
                                        }} />
                                    )}

                                    {/* Start Thumb */}
                                    <input
                                        type="range"
                                        min={0}
                                        max={dur || 1}
                                        step={0.1}
                                        value={photo.TrimStart}
                                        onChange={(e) => handleSliderChange(e, true)}
                                        style={{ position: 'absolute', width: '100%', background: 'transparent', zIndex: 2 }}
                                        className="trim-slider"
                                    />

                                    {/* End Thumb */}
                                    <input
                                        type="range"
                                        min={0}
                                        max={dur || 1}
                                        step={0.1}
                                        value={effectiveEnd}
                                        onChange={(e) => handleSliderChange(e, false)}
                                        style={{ position: 'absolute', width: '100%', background: 'transparent', zIndex: 3 }}
                                        className="trim-slider end-slider"
                                    />
                                </div>

                                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.7rem', color: 'var(--text-muted)' }}>
                                    <span>Start: {photo.TrimStart.toFixed(1)}s</span>
                                    <span>End: {effectiveEnd.toFixed(1)}s</span>
                                </div>
                            </>
                        );
                    })()}
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
