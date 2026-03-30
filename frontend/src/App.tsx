import { useState, useEffect, useRef, useCallback } from 'react';
import { Sidebar } from './components/Sidebar';
import { Grid } from './components/Grid';
import { Viewer } from './components/Viewer';
import { SettingsModal } from './components/SettingsModal';
import { AboutModal } from './components/AboutModal';
import { HelpModal } from './components/HelpModal';
import { CloudSourceModal } from './components/CloudSourceModal';
import { DeviceImportModal } from './components/DeviceImportModal';
import { UpdateToast } from './components/UpdateToast';
import { WhatsNewModal } from './components/WhatsNewModal';
import { SelectDirectory, ScanDirectory, ScanAndDeduplicate, CancelDeduplicate, GetExportedStatus, GetSelections, ToggleSelection, ExportPhotos, SetPhotoRating, GetRatingsForDirectory, CheckDedupStatus, PreloadThumbnails, GetAppConfig, ShouldShowWhatsNew } from '../wailsjs/go/app/App';
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
    const [dedupeStartTime, setDedupeStartTime] = useState<number | null>(null);
    const [elapsedTime, setElapsedTime] = useState('');
    const [theme, setTheme] = useState<string>('dark');
    const [sysMetrics, setSysMetrics] = useState<SystemMetrics | null>(null);
    const [exportSuccess, setExportSuccess] = useState<string | null>(null);
    const [ratings, setRatings] = useState<Record<string, number>>({});
    const [dedupCompleted, setDedupCompleted] = useState(false);
    const [thumbProgress, setThumbProgress] = useState<{current: number, total: number, heicCount?: number, heicDecoder?: string} | null>(null);
    const [settingsOpen, setSettingsOpen] = useState(false);
    const [aboutOpen, setAboutOpen] = useState(false);
    const [helpOpen, setHelpOpen] = useState(false);
    const [cloudOpen, setCloudOpen] = useState(false);
    const [deviceImportOpen, setDeviceImportOpen] = useState(false);
    const [whatsNewOpen, setWhatsNewOpen] = useState(false);
    const [probe, setProbe] = useState<{ OS: string } | undefined>(undefined);
    const [deviceToast, setDeviceToast] = useState<{ name: string; serial: string } | null>(null);
    const [gridColumns, setGridColumns] = useState(1);

    useEffect(() => {
        EventsOn('sys-metrics', (data: any) => {
            setSysMetrics(data);
        });
        return () => { EventsOff('sys-metrics'); };
    }, []);

    // Load probe info (OS detection for conditional UI)
    useEffect(() => {
        GetAppConfig().then(cfg => {
            if (cfg?.probe) setProbe(cfg.probe);
        }).catch(console.error);
        ShouldShowWhatsNew().then(show => {
            if (show) setWhatsNewOpen(true);
        }).catch(console.error);
    }, []);

    // Device auto-detect toast
    useEffect(() => {
        const connectHandler = (data: any) => {
            console.log('[device] auto-detected:', data);
            setDeviceToast({ name: data?.name || 'Device', serial: data?.serial || '' });
        };
        const disconnectHandler = () => {
            setDeviceToast(null);
        };
        EventsOn('device-connected', connectHandler);
        EventsOn('device-disconnected', disconnectHandler);
        return () => {
            EventsOff('device-connected');
            EventsOff('device-disconnected');
        };
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

    // Track elapsed time while deduplicating
    useEffect(() => {
        if (isDeduplicating) {
            setDedupeStartTime(Date.now());
            const interval = setInterval(() => {
                setDedupeStartTime(prev => {
                    if (!prev) return prev;
                    const secs = Math.floor((Date.now() - prev) / 1000);
                    if (secs < 60) {
                        setElapsedTime(`${secs}s elapsed`);
                    } else {
                        const mins = Math.floor(secs / 60);
                        const s = secs % 60;
                        setElapsedTime(`${mins}m ${s}s elapsed`);
                    }
                    return prev;
                });
            }, 1000);
            return () => clearInterval(interval);
        } else {
            setDedupeStartTime(null);
            setElapsedTime('');
        }
    }, [isDeduplicating]);

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
            } else if (e.key === 'ArrowRight') {
                e.preventDefault();
                if (photos.length > 0) {
                    const currentIndex = activePhotoPath ? photos.findIndex(p => p.Path === activePhotoPath) : -1;
                    if (currentIndex < photos.length - 1) {
                        const nextPhoto = photos[currentIndex + 1];
                        setActivePhotoPath(nextPhoto.Path);
                        setActivePhotoDebounced(nextPhoto);
                        document.getElementById(`thumb-${nextPhoto.Path.replace(/[^a-zA-Z0-9]/g, '-')}`)
                            ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                    }
                }
            } else if (e.key === 'ArrowLeft') {
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
            } else if (e.key === 'ArrowDown') {
                e.preventDefault();
                if (photos.length > 0) {
                    const currentIndex = activePhotoPath ? photos.findIndex(p => p.Path === activePhotoPath) : -1;
                    const targetIndex = Math.min(currentIndex + gridColumns, photos.length - 1);
                    if (targetIndex !== currentIndex) {
                        const nextPhoto = photos[targetIndex];
                        setActivePhotoPath(nextPhoto.Path);
                        setActivePhotoDebounced(nextPhoto);
                        document.getElementById(`thumb-${nextPhoto.Path.replace(/[^a-zA-Z0-9]/g, '-')}`)
                            ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                    }
                }
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                if (photos.length > 0) {
                    const currentIndex = activePhotoPath ? photos.findIndex(p => p.Path === activePhotoPath) : -1;
                    const targetIndex = currentIndex - gridColumns;
                    if (targetIndex >= 0) {
                        const prevPhoto = photos[targetIndex];
                        setActivePhotoPath(prevPhoto.Path);
                        setActivePhotoDebounced(prevPhoto);
                        document.getElementById(`thumb-${prevPhoto.Path.replace(/[^a-zA-Z0-9]/g, '-')}`)
                            ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
                    }
                }
            } else if (e.key >= '1' && e.key <= '5') {
                if (activePhotoPath) {
                    const star = parseInt(e.key, 10);
                    const current = ratings[activePhotoPath] || 0;
                    const newRating = current === star ? 0 : star;
                    setRatings(prev => ({ ...prev, [activePhotoPath]: newRating }));
                    SetPhotoRating(activePhotoPath, newRating).catch(err =>
                        console.error('Failed to save rating:', err)
                    );
                }
            }
        };
        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [activePhotoPath, selectedPaths, photos, currentDir, ratings, setActivePhotoDebounced, gridColumns]);

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
        EventsOff('video-duration-ready');
        EventsOff('video-duration-complete');
        setLoading(true);
        setCurrentDir(dir);
        setThumbProgress(null);
        try {
            // Phase 1: Quick scan — show photos immediately with original paths
            const quickPhotos = await ScanDirectory(dir);
            setPhotos(quickPhotos || []);

            EventsOn('video-duration-ready', (data: any) => {
                const { path, duration } = data;
                setPhotos(prev => prev.map(p =>
                    p.Path === path ? appModel.Photo.createFrom({ ...p, Duration: duration }) : p
                ));
            });

            EventsOn('video-duration-complete', () => {
                EventsOff('video-duration-ready');
                EventsOff('video-duration-complete');
            });

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

                    {/* Animated scanner */}
                    <div className="scanner-animation">
                        <div className="scanner-beam"></div>
                    </div>

                    {/* Stage message with pulsing indicator */}
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                        <div style={{
                            width: 8, height: 8, borderRadius: '50%',
                            background: 'var(--accent)',
                            animation: 'pulse 1.5s ease-in-out infinite',
                        }} />
                        <h2 style={{
                            color: 'var(--text-primary)',
                            fontSize: '1.1rem',
                            fontWeight: 600,
                            margin: 0,
                        }}>
                            {dedupeProgress?.message || 'Preparing...'}
                        </h2>
                    </div>

                    {/* Progress bar */}
                    {dedupeProgress && dedupeProgress.total > 0 && (
                        <>
                            <div className="progress-bar-container-large">
                                <div className="progress-bar-fill-large" style={{
                                    width: `${Math.min(100, (dedupeProgress.current / dedupeProgress.total) * 100)}%`,
                                    transition: 'width 0.3s ease-out',
                                }} />
                            </div>
                            <p style={{ color: 'var(--text-secondary)', fontSize: '0.8rem', margin: '6px 0 0' }}>
                                {dedupeProgress.current} / {dedupeProgress.total}
                            </p>
                        </>
                    )}

                    {/* Elapsed time + ETA */}
                    <p style={{ color: 'var(--text-muted, var(--text-secondary))', fontSize: '0.75rem', margin: '4px 0 16px', fontStyle: 'italic' }}>
                        {elapsedTime}{eta ? ` · ${eta}` : ''}
                    </p>

                    <button className="btn btn-primary" style={{ backgroundColor: 'var(--danger)', borderColor: 'var(--danger)', padding: '8px 24px' }} onClick={handleCancelDeduplicate}>
                        Cancel
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
                onOpenAbout={() => setAboutOpen(true)}
                onOpenHelp={() => setHelpOpen(true)}
                onOpenCloud={() => setCloudOpen(true)}
                onOpenDeviceImport={() => setDeviceImportOpen(true)}
                probe={probe}
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
                    onColumnsChange={setGridColumns}
                />

                <Viewer photo={activePhoto} onTrimChange={handleTrimChange} isSelected={selectedPaths.has(activePhotoPath || '')} />
            </div>

            <div className="status-bar" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', width: '100%', paddingRight: '1rem' }}>
                <div style={{ flex: 1 }}>
                    {loading ? (
                        <div className="progress-container"><div className="progress-bar-indeterminate"></div></div>
                    ) : (
                <span>{photos.length} photos • {selectedPaths.size} selected{thumbProgress ? ` • Loading thumbnails ${thumbProgress.current}/${thumbProgress.total}` : ''}{thumbProgress && thumbProgress.heicCount && thumbProgress.heicCount > 0 ? (
                    <span style={{ color: thumbProgress.heicDecoder === 'sips' ? '#a78bfa' : '#d4a017', marginLeft: '6px' }}>
                        {thumbProgress.heicDecoder === 'sips'
                            ? `${thumbProgress.heicCount} HEIC via native decoder`
                            : `${thumbProgress.heicCount} HEIC via FFmpeg (slower)`}
                    </span>
                ) : null}</span>
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
            {aboutOpen && <AboutModal onClose={() => setAboutOpen(false)} />}
            {helpOpen && <HelpModal onClose={() => setHelpOpen(false)} />}
            {cloudOpen && <CloudSourceModal onClose={() => setCloudOpen(false)} onLoadDir={loadDirectory} />}
            {deviceImportOpen && <DeviceImportModal onClose={() => setDeviceImportOpen(false)} onLoadDir={loadDirectory} />}
            {whatsNewOpen && <WhatsNewModal onClose={() => setWhatsNewOpen(false)} />}

            {/* Device auto-detect toast */}
            {deviceToast && (
                <div style={{
                    position: 'fixed',
                    bottom: 40,
                    right: 20,
                    zIndex: 10000,
                    background: 'rgba(30, 30, 40, 0.95)',
                    backdropFilter: 'blur(12px)',
                    border: '1px solid rgba(129, 140, 248, 0.3)',
                    borderRadius: 12,
                    padding: '12px 16px',
                    display: 'flex',
                    alignItems: 'center',
                    gap: 12,
                    boxShadow: '0 8px 32px rgba(0,0,0,0.4)',
                    maxWidth: 360,
                }}>
                    <div style={{
                        width: 36, height: 36, borderRadius: '50%',
                        background: 'rgba(129, 140, 248, 0.15)',
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        flexShrink: 0,
                    }}>
                        <span style={{ fontSize: '1.1rem' }}>&#128241;</span>
                    </div>
                    <div style={{ flex: 1 }}>
                        <div style={{ fontWeight: 600, fontSize: '0.85rem', color: 'var(--text-primary, #fff)' }}>
                            {deviceToast.name} connected
                        </div>
                        <div style={{ fontSize: '0.72rem', color: 'var(--text-secondary, #aaa)', marginTop: 2 }}>
                            Import photos directly to CullSnap
                        </div>
                    </div>
                    <div style={{ display: 'flex', gap: 6, flexShrink: 0 }}>
                        <button
                            className="btn btn-gradient"
                            style={{ fontSize: '0.72rem', padding: '4px 10px' }}
                            onClick={() => { setDeviceToast(null); setDeviceImportOpen(true); }}
                        >
                            Import
                        </button>
                        <button
                            className="btn"
                            style={{ fontSize: '0.72rem', padding: '4px 8px' }}
                            onClick={() => setDeviceToast(null)}
                        >
                            Dismiss
                        </button>
                    </div>
                </div>
            )}

            <UpdateToast />
        </div>
    );
}

export default App;
