import { Check, Star } from 'lucide-react';
import { model } from '../../wailsjs/go/models';
import { useRef, useCallback } from 'react';

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
            <div className="photo-grid">
                {photos.map((photo) => {
                    const isSelected = selectedPaths.has(photo.Path);
                    const isExported = exportedPaths.has(photo.Path);
                    const isActive = activePhoto?.Path === photo.Path;
                    const rating = ratings[photo.Path] || 0;
                    // Use cached thumbnail if available, else original path
                    const imgSrc = photo.ThumbnailPath || photo.Path;

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

                            {/* Show image with lazy loading (works with CSS Grid) */}
                            <img
                                src={imgSrc}
                                alt={photo.Path.split('/').pop()}
                                className="thumbnail-image"
                                loading="lazy"
                                decoding="async"
                                onLoad={(e) => {
                                    (e.target as HTMLImageElement).closest('.thumbnail-card')?.classList.add('loaded');
                                }}
                            />

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
                    <div className="photo-grid">
                        {duplicateGroups.flat().map((photo) => {
                            const isActive = activePhoto?.Path === photo.Path;
                            const imgSrc = photo.ThumbnailPath || photo.Path;
                            return (
                                <div
                                    id={`thumb-${photo.Path.replace(/[^a-zA-Z0-9]/g, '-')}`}
                                    key={photo.Path}
                                    className={`thumbnail-card ${isActive ? 'active' : ''}`}
                                    onClick={() => onPhotoClick(photo)}
                                    style={{ opacity: 0.55 }}
                                >
                                    <img
                                        src={imgSrc}
                                        alt={photo.Path.split('/').pop()}
                                        className="thumbnail-image"
                                        loading="lazy"
                                        decoding="async"
                                        onLoad={(e) => {
                                            (e.target as HTMLImageElement).closest('.thumbnail-card')?.classList.add('loaded');
                                        }}
                                    />
                                </div>
                            );
                        })}
                    </div>
                </details>
            )}
        </div>
    );
}
