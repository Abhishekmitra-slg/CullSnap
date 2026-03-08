import { useState, useEffect } from 'react';
import { Sidebar } from './components/Sidebar';
import { Grid } from './components/Grid';
import { Viewer } from './components/Viewer';
import { SelectDirectory, ScanDirectory, GetExportedStatus, ToggleSelection, ExportPhotos, GetSystemResources } from '../wailsjs/go/main/App';
import { main as appMain, model as appModel } from '../wailsjs/go/models';

function App() {
    const [photos, setPhotos] = useState<appModel.Photo[]>([]);
    const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set());
    const [exportedPaths, setExportedPaths] = useState<Set<string>>(new Set());
    const [currentDir, setCurrentDir] = useState<string>('');
    const [activePhoto, setActivePhoto] = useState<appModel.Photo | null>(null);
    const [loading, setLoading] = useState(false);
    const [sysMetrics, setSysMetrics] = useState<appMain.SystemResources | null>(null);
    const [exportSuccess, setExportSuccess] = useState<string | null>(null);

    useEffect(() => {
        const interval = setInterval(async () => {
            try {
                const metrics = await GetSystemResources();
                setSysMetrics(metrics);
            } catch (e) {
                console.error("Failed to fetch metrics", e);
            }
        }, 1000);
        return () => clearInterval(interval);
    }, []);

    // Keyboard shortcut listener
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.key === 's' || e.key === 'S') {
                if (activePhoto) {
                    handleToggleSelection(activePhoto.Path);
                }
            } else if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
                e.preventDefault();
                if (photos.length > 0) {
                    const currentIndex = activePhoto ? photos.findIndex(p => p.Path === activePhoto.Path) : -1;
                    if (currentIndex < photos.length - 1) {
                        setActivePhoto(photos[currentIndex + 1]);
                        // Try to scroll the thumbnail into view if possible
                        const id = photos[currentIndex + 1].Path.replace(/[^a-zA-Z0-9]/g, '-');
                        document.getElementById(`thumb-${id}`)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                    }
                }
            } else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
                e.preventDefault();
                if (photos.length > 0) {
                    const currentIndex = activePhoto ? photos.findIndex(p => p.Path === activePhoto.Path) : -1;
                    if (currentIndex > 0) {
                        setActivePhoto(photos[currentIndex - 1]);
                        const id = photos[currentIndex - 1].Path.replace(/[^a-zA-Z0-9]/g, '-');
                        document.getElementById(`thumb-${id}`)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                    }
                }
            }
        };
        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [activePhoto, selectedPaths, photos, currentDir]);

    const handleOpenFolder = async () => {
        try {
            const dir = await SelectDirectory();
            if (dir) {
                loadDirectory(dir);
            }
        } catch (e) {
            console.error(e);
        }
    };

    const handleExportSuccess = (msg: string) => {
        setExportSuccess(msg);
        setTimeout(() => setExportSuccess(null), 3000);
    };

    const loadDirectory = async (dir: string) => {
        setLoading(true);
        setCurrentDir(dir);
        try {
            const loadedPhotos = await ScanDirectory(dir);
            setPhotos(loadedPhotos || []);

            const exportedStatus = await GetExportedStatus(dir);
            setExportedPaths(new Set(Object.keys(exportedStatus || {})));

            if (loadedPhotos && loadedPhotos.length > 0) {
                setActivePhoto(loadedPhotos[0]);
            } else {
                setActivePhoto(null);
            }
            setSelectedPaths(new Set());
        } catch (e) {
            console.error(e);
        } finally {
            setLoading(false);
        }
    };

    const handleToggleSelection = async (path: string) => {
        const newSelected = new Set(selectedPaths);
        const isSelected = !newSelected.has(path);
        if (isSelected) {
            newSelected.add(path);
        } else {
            newSelected.delete(path);
        }
        setSelectedPaths(newSelected);
        try {
            await ToggleSelection(path, currentDir, isSelected);
        } catch (e) {
            console.error(e);
            // Revert on error
            const revertSelected = new Set(newSelected);
            isSelected ? revertSelected.delete(path) : revertSelected.add(path);
            setSelectedPaths(revertSelected);
        }
    };

    const handlePhotoClick = (photo: appModel.Photo) => {
        setActivePhoto(photo);
    };

    return (
        <div id="App" className="app-container">
            <div className="titlebar" />

            <Sidebar
                currentDir={currentDir}
                photosCount={photos.length}
                selectedCount={selectedPaths.size}
                onOpenFolder={handleOpenFolder}
                onLoadDir={loadDirectory}
                photos={photos}
                selectedPaths={selectedPaths}
                onExportSuccess={handleExportSuccess}
            />

            <div className="main-content">
                <Grid
                    photos={photos}
                    selectedPaths={selectedPaths}
                    exportedPaths={exportedPaths}
                    activePhoto={activePhoto}
                    onPhotoClick={handlePhotoClick}
                />

                <Viewer photo={activePhoto} />
            </div>

            <div className="status-bar" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', width: '100%', paddingRight: '1rem' }}>
                <div style={{ flex: 1 }}>
                    {loading ? (
                        <div className="progress-container"><div className="progress-bar-indeterminate"></div></div>
                    ) : (
                        <span>{photos.length} photos • {selectedPaths.size} selected</span>
                    )}
                </div>

                {sysMetrics && (
                    <div className="sys-metrics flex items-center gap-3" style={{ background: 'transparent', border: 'none', boxShadow: 'none' }}>
                        <span title="CPU Usage">CPU: {sysMetrics.cpu.toFixed(1)}%</span>
                        <span title="Backend Engine Memory Usage">Engine RAM: {sysMetrics.ram.toFixed(0)} MB</span>
                        <span title="Disk Read/Write">Disk: {(sysMetrics.diskRead || 0).toFixed(1)}/{(sysMetrics.diskWrite || 0).toFixed(1)} MB/s</span>
                        <span title="Network Send/Recv">Net: {(sysMetrics.netSent || 0).toFixed(1)}/{(sysMetrics.netRecv || 0).toFixed(1)} KB/s</span>
                    </div>
                )}
            </div>

            {/* Export Success Toast overlay */}
            {exportSuccess && (
                <div className="export-toast">
                    {exportSuccess}
                </div>
            )}
        </div>
    );
}

export default App;
