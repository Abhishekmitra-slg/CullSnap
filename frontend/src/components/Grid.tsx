import { Check } from 'lucide-react';
import { model } from '../../wailsjs/go/models';
import { useVirtualizer } from '@tanstack/react-virtual';
import { useRef, useEffect } from 'react';

interface GridProps {
    photos: model.Photo[];
    duplicateGroups?: model.Photo[][];
    selectedPaths: Set<string>;
    exportedPaths: Set<string>;
    activePhoto: model.Photo | null;
    onPhotoClick: (photo: model.Photo) => void;
}

export function Grid({
    photos,
    duplicateGroups,
    selectedPaths,
    exportedPaths,
    activePhoto,
    onPhotoClick
}: GridProps) {
    const parentRef = useRef<HTMLDivElement>(null);

    // Number of columns in the grid (responsive could be dynamic, hardcode to 4 for simplicity first)
    const columns = 4;
    const count = Math.ceil(photos.length / columns);

    const rowVirtualizer = useVirtualizer({
        count,
        getScrollElement: () => parentRef.current,
        estimateSize: () => 180, // estimated height of row + gap
        overscan: 5,
    });

    if (photos.length === 0 && (!duplicateGroups || duplicateGroups.length === 0)) {
        return (
            <div className="grid-panel" style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--text-secondary)' }}>
                <p>No photos loaded. Open a folder to begin culling.</p>
            </div>
        );
    }

    return (
        <div className="grid-panel" ref={parentRef} style={{ overflowY: 'auto' }}>
            <div
                className="photo-grid-virtual"
                style={{
                    height: `${rowVirtualizer.getTotalSize()}px`,
                    width: '100%',
                    position: 'relative',
                }}
            >
                {rowVirtualizer.getVirtualItems().map((virtualRow) => {
                    const rowPhotos = photos.slice(
                        virtualRow.index * columns,
                        (virtualRow.index + 1) * columns
                    );

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
                                display: 'grid',
                                gridTemplateColumns: `repeat(${columns}, 1fr)`,
                                gap: '1rem',
                                padding: '1rem'
                            }}
                        >
                            {rowPhotos.map((photo) => {
                                const isSelected = selectedPaths.has(photo.Path);
                                const isExported = exportedPaths.has(photo.Path);
                                const isActive = activePhoto?.Path === photo.Path;

                                return (
                                    <div
                                        id={`thumb-${photo.Path.replace(/[^a-zA-Z0-9]/g, '-')}`}
                                        key={photo.Path}
                                        className={`thumbnail-card ${isSelected ? 'selected' : ''} ${isActive ? 'active' : ''}`}
                                        onClick={() => onPhotoClick(photo)}
                                        style={{ height: '160px', width: '100%' }} // Fixed height for virtual rows
                                    >
                                        {isSelected && (
                                            <div className="badge badge-selected bg-blue-500">
                                                <Check size={14} />
                                            </div>
                                        )}
                                        {!isSelected && isExported && (
                                            <div className="badge badge-exported bg-green-500">
                                                <Check size={14} />
                                            </div>
                                        )}

                                        <img
                                            src={photo.Path}
                                            alt={photo.Path.split('/').pop()}
                                            className="thumbnail-image"
                                            loading="lazy"
                                            decoding="async"
                                            style={{ objectFit: 'cover', width: '100%', height: '100%', borderRadius: '0.5rem' }}
                                            onLoad={(e) => {
                                                (e.target as HTMLImageElement).classList.add('loaded');
                                            }}
                                        />
                                    </div>
                                );
                            })}
                        </div>
                    );
                })}
            </div>

            {/* Collapsible Duplicates Section */}
            {duplicateGroups && duplicateGroups.length > 0 && (
                <details className="mt-8 mb-8 mx-4" style={{ cursor: 'pointer' }}>
                    <summary className="text-large text-gray-400 font-semibold mb-4" style={{ opacity: 0.8 }}>
                        Hidden Duplicates ({duplicateGroups.reduce((acc, g) => acc + g.length, 0)})
                    </summary>
                    <div style={{ display: 'grid', gridTemplateColumns: `repeat(${columns}, 1fr)`, gap: '1rem' }}>
                        {duplicateGroups.flat().map((photo) => {
                            const isActive = activePhoto?.Path === photo.Path;
                            return (
                                <div
                                    id={`thumb-${photo.Path.replace(/[^a-zA-Z0-9]/g, '-')}`}
                                    key={photo.Path}
                                    className={`thumbnail-card ${isActive ? 'active' : ''}`}
                                    onClick={() => onPhotoClick(photo)}
                                    style={{ height: '160px', width: '100%', opacity: 0.6 }} // Dim duplicates
                                >
                                    <img
                                        src={photo.Path}
                                        alt={photo.Path.split('/').pop()}
                                        className="thumbnail-image"
                                        loading="lazy"
                                        decoding="async"
                                        style={{ objectFit: 'cover', width: '100%', height: '100%', borderRadius: '0.5rem' }}
                                        onLoad={(e) => {
                                            (e.target as HTMLImageElement).classList.add('loaded');
                                        }}
                                    />
                                    {/* Small duplicate badge */}
                                    <div className="badge" style={{ background: 'var(--bg-panel)', color: 'var(--text-secondary)' }}>
                                        Duplicate
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                </details>
            )}
        </div>
    );
}
