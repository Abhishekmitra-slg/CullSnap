import { model } from '../../wailsjs/go/models';

interface ViewerProps {
    photo: model.Photo | null;
}

export function Viewer({ photo }: ViewerProps) {
    if (!photo) {
        return (
            <div className="viewer-panel">
                <div style={{ color: 'var(--text-secondary)' }}>Select a photo to view</div>
            </div>
        );
    }

    const filename = photo.Path.split('/').pop();

    // Format sizes
    const kbSize = (photo.Size / 1024).toFixed(1);
    const mbSize = (photo.Size / 1024 / 1024).toFixed(1);
    const sizeText = photo.Size > 1024 * 1024 ? `${mbSize} MB` : `${kbSize} KB`;

    return (
        <div className="viewer-panel">
            <img
                src={photo.Path}
                alt={filename}
                className="viewer-image"
            />
            <div className="glass-panel" style={{
                position: 'absolute',
                top: '1rem',
                right: '1rem',
                padding: '0.75rem 1rem',
                fontSize: '0.875rem',
                display: 'flex',
                flexDirection: 'column',
                gap: '0.25rem',
                minWidth: '200px'
            }}>
                <div style={{ fontWeight: 600, color: 'white', wordBreak: 'break-all' }}>{filename}</div>
                <div style={{ color: 'var(--text-secondary)' }}>{sizeText}</div>
            </div>
        </div>
    );
}
