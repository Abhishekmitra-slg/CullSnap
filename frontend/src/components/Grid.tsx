import { Check, Star } from 'lucide-react';
import { model } from '../../wailsjs/go/models';
import { useRef, useCallback, useState, useEffect } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';

interface GridProps {
    photos: model.Photo[];
    duplicateGroups?: model.Photo[][];
    selectedPaths: Set<string>;
    exportedPaths: Set<string>;
    activePhotoPath: string;
    onPhotoClick: (photo: model.Photo) => void;
    ratings: Record<string, number>;
    onRatingChange: (path: string, rating: number) => void;
}

function chunk<T>(arr: T[], size: number): T[][] {
    const result: T[][] = [];
    for (let i = 0; i < arr.length; i += size) {
        result.push(arr.slice(i, i + size));
    }
    return result;
}

export function Grid({
    photos,
    duplicateGroups,
    selectedPaths,
    exportedPaths,
    activePhotoPath,
    onPhotoClick,
    ratings,
    onRatingChange,
}: GridProps) {
    const parentRef = useRef<HTMLDivElement>(null);

    const [containerWidth, setContainerWidth] = useState(800);

    useEffect(() => {
        if (!parentRef.current) return;
        const observer = new ResizeObserver(entries => {
            for (const entry of entries) {
                setContainerWidth(entry.contentRect.width);
            }
        });
        observer.observe(parentRef.current);
        return () => observer.disconnect();
    }, []);

    const CARD_WIDTH = 168; // 160px card + 8px gap
    const CARD_HEIGHT = 176; // 160px card + 16px gap
    const columns = Math.max(1, Math.floor(containerWidth / CARD_WIDTH));
    const rows = chunk(photos, columns);

    const rowVirtualizer = useVirtualizer({
        count: rows.length,
        getScrollElement: () => parentRef.current,
        estimateSize: () => CARD_HEIGHT,
        overscan: 3,
    });

    useEffect(() => {
        if (!activePhotoPath) return;
        const idx = photos.findIndex(p => p.Path === activePhotoPath);
        if (idx === -1) return;
        const rowIndex = Math.floor(idx / columns);
        rowVirtualizer.scrollToIndex(rowIndex, { align: 'auto' });
    }, [columns, activePhotoPath, photos]);

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
        <div className="grid-panel" ref={parentRef} style={{ overflowY: 'auto' }}>
            <div
                style={{
                    height: `${rowVirtualizer.getTotalSize()}px`,
                    width: '100%',
                    position: 'relative',
                }}
            >
                {rowVirtualizer.getVirtualItems().map(virtualRow => {
                    const rowPhotos = rows[virtualRow.index];
                    return (
                        <div
                            key={virtualRow.index}
                            style={{
                                position: 'absolute',
                                top: 0,
                                left: 0,
                                width: '100%',
                                height: `${virtualRow.size}px`,
                                transform: `translateY(${virtualRow.start}px)`,
                                display: 'flex',
                                gap: 8,
                            }}
                        >
                            {rowPhotos.map((photo) => {
                                const isSelected = selectedPaths.has(photo.Path);
                                const isExported = exportedPaths.has(photo.Path);
                                const isActive = activePhotoPath === photo.Path;
                                const rating = ratings[photo.Path] || 0;
                                const rawSrc = photo.ThumbnailPath || photo.Path;
                                const imgSrc = `http://localhost:34342/wails-media?path=${encodeURIComponent(rawSrc)}`;

                                return (
                                    <div
                                        id={`thumb-${photo.Path.replace(/[^a-zA-Z0-9]/g, '-')}`}
                                        key={photo.Path}
                                        className={`thumbnail-card ${isSelected ? 'selected' : ''} ${isActive ? 'active' : ''}`}
                                        onClick={() => onPhotoClick(photo)}
                                    >
                                        {isSelected && <div className="badge badge-selected"><Check size={12} /></div>}
                                        {!isSelected && isExported && <div className="badge badge-exported"><Check size={12} /></div>}
                                        {photo.IsVideo && (
                                            <div className="badge badge-video">
                                                <span style={{ fontSize: '10px' }}>
                                                    {Math.floor(photo.Duration / 60)}:{(Math.floor(photo.Duration % 60)).toString().padStart(2, '0')}
                                                </span>
                                            </div>
                                        )}
                                        {photo.isRAW && photo.rawFormat && (
                                            <div className="badge-raw">
                                                {photo.rawFormat}
                                            </div>
                                        )}
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
                            const isActive = activePhotoPath === photo.Path;
                            const rawSrc = photo.ThumbnailPath || photo.Path;
                            const imgSrc = `http://localhost:34342/wails-media?path=${encodeURIComponent(rawSrc)}`;
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
