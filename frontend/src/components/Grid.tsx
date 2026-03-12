import { Check, Star } from 'lucide-react';
import { model } from '../../wailsjs/go/models';
import { useRef, useCallback, useState, useEffect } from 'react';

interface GridProps {
    photos: model.Photo[];
    duplicateGroups?: model.Photo[][];
    selectedPaths: Set<string>;
    exportedPaths: Set<string>;
    activePhoto: model.Photo | null;
    onPhotoClick: (photo: model.Photo) => void;
    ratings: Record<string, number>;
    onRatingChange: (path: string, rating: number) => void;
}

// Lazy-loading thumbnail component using IntersectionObserver
function LazyThumbnail({ path, alt }: { path: string; alt: string }) {
    const imgRef = useRef<HTMLImageElement>(null);
    const [src, setSrc] = useState<string>('');
    const [isVisible, setIsVisible] = useState(false);

    useEffect(() => {
        const img = imgRef.current;
        if (!img) return;

        const observer = new IntersectionObserver(
            (entries) => {
                entries.forEach((entry) => {
                    if (entry.isIntersecting) {
                        setIsVisible(true);
                        observer.unobserve(entry.target);
                    }
                });
            },
            { rootMargin: '200px' } // Preload 200px ahead
        );

        observer.observe(img);
        return () => observer.disconnect();
    }, []);

    useEffect(() => {
        if (!isVisible) return;

        let cancelled = false;
        const loadThumb = async () => {
            try {
                const { GetThumbnailBase64 } = await import('../../wailsjs/go/app/App');
                const data = await GetThumbnailBase64(path);
                if (!cancelled && data) {
                    setSrc(data);
                }
            } catch {
                // Fallback to original path if thumbnail generation fails
                if (!cancelled) setSrc(path);
            }
        };
        loadThumb();
        return () => { cancelled = true; };
    }, [isVisible, path]);

    return (
        <img
            ref={imgRef}
            src={src || undefined}
            alt={alt}
            className={`thumbnail-image ${src ? 'loaded' : ''}`}
            decoding="async"
        />
    );
}

export function Grid({
    photos,
    duplicateGroups,
    selectedPaths,
    exportedPaths,
    activePhoto,
    onPhotoClick,
    ratings,
    onRatingChange,
}: GridProps) {
    const parentRef = useRef<HTMLDivElement>(null);

    const handleStarClick = useCallback((e: React.MouseEvent, path: string, star: number) => {
        e.stopPropagation();
        const current = ratings[path] || 0;
        onRatingChange(path, current === star ? 0 : star);
    }, [ratings, onRatingChange]);

    if (photos.length === 0 && (!duplicateGroups || duplicateGroups.length === 0)) {
        return (
            <div className="grid-panel" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <div style={{ textAlign: 'center', color: 'var(--text-muted)' }}>
                    <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" style={{ opacity: 0.4 }}>
                        <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
                    </svg>
                    <p style={{ marginTop: 12, fontSize: '0.875rem' }}>No photos loaded</p>
                    <p style={{ fontSize: '0.75rem', marginTop: 4 }}>Open a folder to begin culling</p>
                </div>
            </div>
        );
    }

    return (
        <div className="grid-panel" ref={parentRef}>
            <div className="masonry-grid">
                {photos.map((photo) => {
                    const isSelected = selectedPaths.has(photo.Path);
                    const isExported = exportedPaths.has(photo.Path);
                    const isActive = activePhoto?.Path === photo.Path;
                    const rating = ratings[photo.Path] || 0;

                    return (
                        <div
                            id={`thumb-${photo.Path.replace(/[^a-zA-Z0-9]/g, '-')}`}
                            key={photo.Path}
                            className={`thumbnail-card ${isSelected ? 'selected' : ''} ${isActive ? 'active' : ''}`}
                            onClick={() => onPhotoClick(photo)}
                        >
                            {isSelected && (
                                <div className="badge badge-selected">
                                    <Check size={12} />
                                </div>
                            )}
                            {!isSelected && isExported && (
                                <div className="badge badge-exported">
                                    <Check size={12} />
                                </div>
                            )}

                            <LazyThumbnail path={photo.Path} alt={photo.Path.split('/').pop() || ''} />

                            {/* Star rating overlay */}
                            <div className="star-rating">
                                {[1, 2, 3, 4, 5].map((star) => (
                                    <Star
                                        key={star}
                                        size={18}
                                        className={`star ${star <= rating ? 'filled' : ''}`}
                                        fill={star <= rating ? '#fbbf24' : 'none'}
                                        onClick={(e) => handleStarClick(e, photo.Path, star)}
                                    />
                                ))}
                            </div>
                        </div>
                    );
                })}
            </div>

            {/* Collapsible Duplicates Section */}
            {duplicateGroups && duplicateGroups.length > 0 && (
                <details className="duplicates-section">
                    <summary>
                        Hidden Duplicates ({duplicateGroups.reduce((acc, g) => acc + g.length, 0)})
                    </summary>
                    <div className="duplicates-grid">
                        {duplicateGroups.flat().map((photo) => {
                            const isActive = activePhoto?.Path === photo.Path;
                            return (
                                <div
                                    id={`thumb-${photo.Path.replace(/[^a-zA-Z0-9]/g, '-')}`}
                                    key={photo.Path}
                                    className={`thumbnail-card ${isActive ? 'active' : ''}`}
                                    onClick={() => onPhotoClick(photo)}
                                    style={{ opacity: 0.55 }}
                                >
                                    <LazyThumbnail path={photo.Path} alt={photo.Path.split('/').pop() || ''} />
                                </div>
                            );
                        })}
                    </div>
                </details>
            )}
        </div>
    );
}
