import { useState, useEffect, useRef, useCallback } from 'react';
import { Sidebar } from './components/Sidebar';
import { Grid } from './components/Grid';
import { Viewer } from './components/Viewer';
import { SettingsModal } from './components/SettingsModal';
import { SelectDirectory, ScanDirectory, ScanAndDeduplicate, CancelDeduplicate, GetExportedStatus, GetSelections, ToggleSelection, ExportPhotos, SetPhotoRating, GetRatingsForDirectory, CheckDedupStatus, PreloadThumbnails } from '../wailsjs/go/app/App';
import { model as appModel } from '../wailsjs/go/models';

interface SystemMetrics {
    cpu: number;
    ram: number;
    diskRead: number;
    diskWrite: number;
    netSent: number;
    netRecv: number;
}
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';

function App() {
    const [photos, setPhotos] = useState<appModel.Photo[]>([]);
    const [duplicateGroups, setDuplicateGroups] = useState<appModel.Photo[][]>([]);
    const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set());
    const [exportedPaths, setExportedPaths] = useState<Set<string>>(new Set());
    const [currentDir, setCurrentDir] = useState<string>('');
    const [activePhoto, setActivePhoto] = useState<appModel.Photo | null>(null);
    const [activePhotoPath, setActivePhotoPath] = useState<string>('');
    const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const [loading, setLoading] = useState(false);
    const [isDeduplicating, setIsDeduplicating] = useState(false);
    const [dedupeProgress, setDedupeProgress] = useState<{current: number, total: number, message: string} | null>(null);
    const [theme, setTheme] = useState<string>('dark');
    const [sysMetrics, setSysMetrics] = useState<SystemMetrics | null>(null);
    const [exportSuccess, setExportSuccess] = useState<string | null>(null);
    const [ratings, setRatings] = useState<Record<string, number>>({});
    const [dedupCompleted, setDedupCompleted] = useState(false);
    const [thumbProgress, setThumbProgress] = useState<{current: number, total: number} | null>(null);
    const [settingsOpen, setSettingsOpen] = useState(false);

    useEffect(() => {
        EventsOn('sys-metrics', (data: any) => {
            setSysMetrics(data);
        });
        return () => { EventsOff('sys-metrics'); };
    }, []);

    // Cleanup debounce timer on unmount
    useEffect(() => {
        return () => {
            if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
        };
    }, []);

    // Listen to deduplication progress events
    const phaseStartRef = useRef<{ time: number; message: string } | null>(null);
    const [eta, setEta] = useState<string | null>(null);

    useEffect(() => {
        const handler = (data: any) => {
            setDedupeProgress(data);

            // Track phase changes for ETA calculation
            if (!phaseStartRef.current || phaseStartRef.current.message !== data.message) {
                phaseStartRef.current = { time: Date.now(), message: data.message };
                setEta(null);
            } else if (data.current > 0 && data.total > 0) {
                const elapsed = (Date.now() - phaseStartRef.current.time) / 1000; // seconds
                const itemsPerSec = data.current / elapsed;
                const remaining = data.total - data.current;
                if (itemsPerSec > 0 && remaining > 0) {
                    const secsLeft = remaining / itemsPerSec;
                    if (secsLeft < 60) {
                        setEta(`~${Math.ceil(secsLeft)}s remaining`);
                    } else {
                        const mins = Math.floor(secsLeft / 60);
                        const secs = Math.ceil(secsLeft % 60);
                        setEta(`~${mins}m ${secs}s remaining`);
                    }
                } else {
                    setEta(null);
                }
            }
        };
        // Use global window.runtime injected by Wails
        if ((window as any).runtime) {
            (window as any).runtime.EventsOn("dedupe-progress", handler);
        }
        return () => {
            if ((window as any).runtime) {
                (window as any).runtime.EventsOff("dedupe-progress");
            }
        };
    }, []);

    const setActivePhotoDebounced = useCallback((photo: appModel.Photo) => {
        if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
        debounceTimerRef.current = setTimeout(() => {
            setActivePhoto(photo);
        }, 80);
    }, []);

    // Keyboard shortcut listener
    useEffect(() => {
        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.key === 's' || e.key === 'S') {
                if (activePhotoPath) {
                    handleToggleSelection(activePhotoPath);
                }
            } else if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
                e.preventDefault();
                if (photos.length > 0) {
                    const currentIndex = activePhotoPath ? photos.findIndex(p => p.Path === activePhotoPath) : -1;
                    if (currentIndex < photos.length - 1) {
                        const nextPhoto = photos[currentIndex + 1];
                        setActivePhotoPath(nextPhoto.Path);          // immediate — grid highlight
                        setActivePhotoDebounced(nextPhoto);           // 80ms — viewer load
                        document.getElementById(`thumb-${nextPhoto.Path.replace(/[^a-zA-Z0-9]/g, '-')}`)
                            ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                    }
                }
            } else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
                e.preventDefault();
                if (photos.length > 0) {
                    const currentIndex = activePhotoPath ? photos.findIndex(p => p.Path === activePhotoPath) : -1;
                    if (currentIndex > 0) {
                        const prevPhoto = photos[currentIndex - 1];
                        setActivePhotoPath(prevPhoto.Path);
                        setActivePhotoDebounced(prevPhoto);
                        document.getElementById(`thumb-${prevPhoto.Path.replace(/[^a-zA-Z0-9]/g, '-')}`)
                            ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                    }
                }
            }
        };
        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [activePhotoPath, selectedPaths, photos, currentDir, setActivePhotoDebounced]);

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

    const handleThemeChange = (newTheme: string) => {
        setTheme(newTheme);
    };

    const loadDirectory = async (dir: string) => {
        setLoading(true);
        setCurrentDir(dir);
        setThumbProgress(null);
        try {
            // Phase 1: Quick scan — show photos immediately with original paths
            const quickPhotos = await ScanDirectory(dir);
            setPhotos(quickPhotos || []);

            const exportedStatus = await GetExportedStatus(dir);
            setExportedPaths(new Set(Object.keys(exportedStatus || {})));

            if (quickPhotos && quickPhotos.length > 0) {
                setActivePhoto(quickPhotos[0]);
                setActivePhotoPath(quickPhotos[0].Path);
            } else {
                setActivePhoto(null);
                setActivePhotoPath('');
            }

            // Restore persisted selections from previous session
            try {
                const savedSelections = await GetSelections(dir);
                setSelectedPaths(new Set(Object.keys(savedSelections || {})));
            } catch {
                setSelectedPaths(new Set());
            }

            setDuplicateGroups([]);
            setDedupCompleted(false);

            // Load star ratings
            try {
                const dirRatings = await GetRatingsForDirectory(dir);
                setRatings(dirRatings || {});
            } catch {
                setRatings({});
            }

            // Auto-detect existing dedup results
            try {
                const status = await CheckDedupStatus(dir);
                if (status && status.hasDuplicates && status.duplicates) {
                    setDuplicateGroups([status.duplicates]);
                    setDedupCompleted(true);
                }
            } catch {
                // No dedup results found, that's fine
            }

            setLoading(false);

            // Phase 2: Preload thumbnails in background (parallel goroutines)
            // Listen for progress events
            const thumbHandler = (data: any) => {
                setThumbProgress(data);
            };
            EventsOn('thumb-progress', thumbHandler);

            try {
                const thumbPhotos = await PreloadThumbnails(dir);
                if (thumbPhotos && thumbPhotos.length > 0) {
                    // Update photos with thumbnail paths
                    setPhotos(thumbPhotos);
                }
            } catch (e) {
                console.error('Thumbnail preload failed:', e);
            } finally {
                EventsOff('thumb-progress');
                setThumbProgress(null);
            }
        } catch (e) {
            console.error(e);
            setLoading(false);
        }
    };

    const handleDeduplicate = async () => {
        if (!currentDir) return;
        setIsDeduplicating(true);
        setDedupeProgress(null);
        try {
            const result = await ScanAndDeduplicate(currentDir, 8);
            if (result) {
                setPhotos(result.uniquePhotos || []);
                setDuplicateGroups(result.duplicateGroups || []);
                if (result.uniquePhotos && result.uniquePhotos.length > 0) {
                    setActivePhoto(result.uniquePhotos[0]);
                    setActivePhotoPath(result.uniquePhotos[0].Path);
                } else {
                    setActivePhoto(null);
                    setActivePhotoPath('');
                }
            }
        } catch (e) {
            console.error("Deduplication failed", e);
            alert(`Deduplication failed: ${e}`);
        } finally {
            setIsDeduplicating(false);
            setDedupeProgress(null);
            setEta(null);
            phaseStartRef.current = null;
        }
    };

    const handleCancelDeduplicate = () => {
        CancelDeduplicate();
        // We do not set isDeduplicating to false here immediately,
        // we let the ScanAndDeduplicate promise resolve/reject to finish cleanup.
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
        setActivePhotoPath(photo.Path);
        setActivePhoto(photo);
    };

    const handleRatingChange = useCallback(async (path: string, rating: number) => {
        setRatings(prev => ({ ...prev, [path]: rating }));
        try {
            await SetPhotoRating(path, rating);
        } catch (e) {
            console.error('Failed to save rating:', e);
        }
    }, []);

    const handleTrimChange = useCallback((path: string, start: number, end: number) => {
        setPhotos(prev => prev.map(p => p.Path === path ? appModel.Photo.createFrom({ ...p, TrimStart: start, TrimEnd: end }) : p));
        setActivePhoto(prev => (prev?.Path === path) ? appModel.Photo.createFrom({ ...prev, TrimStart: start, TrimEnd: end }) : prev);
    }, []);

    return (
        <div id="App" className="app-container" data-theme={theme}>
            <div className="titlebar" />

            {/* FULL SCREEN DEDUPE MODAL OVERLAY */}
            {isDeduplicating && (
                <div className="dedupe-progress-overlay glass-panel flex flex-col items-center justify-center p-4 rounded-none border-0" 
                     style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, zIndex: 9999 }}>
                    <div className="scanner-animation">
                        <div className="scanner-beam"></div>
                    </div>
                    <h2 className="text-xl mb-2" style={{ color: 'var(--text-primary)', textTransform: 'none', letterSpacing: 'normal', fontSize: '1.25rem' }}>
                        {dedupeProgress ? dedupeProgress.message : 'Initializing...'}
                    </h2>
                    
                    {dedupeProgress && dedupeProgress.total > 0 && (
                        <>
                            <div className="progress-bar-container-large">
                                <div className="progress-bar-fill-large" style={{ width: `${(dedupeProgress.current / dedupeProgress.total) * 100}%` }}></div>
                            </div>
                            <p className="text-small mt-2" style={{ color: 'var(--text-secondary)' }}>
                                {dedupeProgress.current} / {dedupeProgress.total}
                            </p>
                            {eta && (
                                <p className="text-small mt-1 mb-4" style={{ color: 'var(--text-muted, var(--text-secondary))', fontStyle: 'italic' }}>
                                    {eta}
                                </p>
                            )}
                        </>
                    )}

                    <button className="btn btn-primary mt-4" style={{ backgroundColor: 'var(--danger)', borderColor: 'var(--danger)' }} onClick={handleCancelDeduplicate}>
                        Abort Process
                    </button>
                </div>
            )}

            <Sidebar
                currentDir={currentDir}
                photosCount={photos.length}
                selectedCount={selectedPaths.size}
                onOpenFolder={handleOpenFolder}
                onLoadDir={loadDirectory}
                onDeduplicate={handleDeduplicate}
                isDeduplicating={isDeduplicating}
                photos={photos}
                selectedPaths={selectedPaths}
                onExportSuccess={handleExportSuccess}
                theme={theme}
                onThemeChange={handleThemeChange}
                dedupCompleted={dedupCompleted}
                duplicateCount={duplicateGroups.reduce((acc, g) => acc + g.length, 0)}
                onOpenSettings={() => setSettingsOpen(true)}
            />

            <div className="main-content" style={{ position: 'relative' }}>
                <Grid
                    photos={photos}
                    duplicateGroups={duplicateGroups}
                    selectedPaths={selectedPaths}
                    exportedPaths={exportedPaths}
                    activePhotoPath={activePhotoPath}
                    onPhotoClick={handlePhotoClick}
                    ratings={ratings}
                    onRatingChange={handleRatingChange}
                />

                <Viewer photo={activePhoto} onTrimChange={handleTrimChange} />
            </div>

            <div className="status-bar" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', width: '100%', paddingRight: '1rem' }}>
                <div style={{ flex: 1 }}>
                    {loading ? (
                        <div className="progress-container"><div className="progress-bar-indeterminate"></div></div>
                    ) : (
                <span>{photos.length} photos • {selectedPaths.size} selected{thumbProgress ? ` • Loading thumbnails ${thumbProgress.current}/${thumbProgress.total}` : ''}</span>
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

            {settingsOpen && <SettingsModal onClose={() => setSettingsOpen(false)} />}
        </div>
    );
}

export default App;
