import { useState, useEffect, useRef } from 'react';
import { X, Cloud, RefreshCw, FolderDown, LogOut, Loader } from 'lucide-react';
import {
    GetCloudSources,
    AuthenticateCloudSource,
    DisconnectCloudSource,
    ListCloudAlbums,
    MirrorCloudAlbum,
    CancelMirror,
} from '../../wailsjs/go/app/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

interface CloudSourceModalProps {
    onClose: () => void;
    onLoadDir: (dir: string) => void;
}

interface CloudProvider {
    providerID: string;
    displayName: string;
    isAvailable: boolean;
    isConnected: boolean;
}

interface CloudAlbum {
    id: string;
    title: string;
    mediaCount: number;
    coverUrl?: string;
}

interface MirrorProgress {
    downloaded: number;
    total: number;
    albumID: string;
    currentFile: string;
    startTime: number;
    phase: string;
}

export function CloudSourceModal({ onClose, onLoadDir }: CloudSourceModalProps) {
    const [providers, setProviders] = useState<CloudProvider[]>([]);
    const [selectedProvider, setSelectedProvider] = useState<string | null>(null);
    const [albums, setAlbums] = useState<CloudAlbum[]>([]);
    const [loadingProviders, setLoadingProviders] = useState(true);
    const [loadingAlbums, setLoadingAlbums] = useState(false);
    const [authenticating, setAuthenticating] = useState<string | null>(null);
    const [mirroring, setMirroring] = useState<string | null>(null);
    const [mirrorProgress, setMirrorProgress] = useState<MirrorProgress | null>(null);
    const [error, setError] = useState<string | null>(null);
    const mountedRef = useRef(true);

    useEffect(() => {
        mountedRef.current = true;
        return () => { mountedRef.current = false; };
    }, []);

    // Load providers on mount
    useEffect(() => {
        loadProviders();
    }, []);

    // Listen for cloud events
    useEffect(() => {
        const authHandler = (data: any) => {
            if (!mountedRef.current) return;
            console.log('[cloud] auth complete:', data);
            setAuthenticating(null);
            loadProviders();
            if (data?.providerID) {
                setSelectedProvider(data.providerID);
                loadAlbums(data.providerID);
            }
        };

        const progressHandler = (data: any) => {
            if (!mountedRef.current) return;
            setMirrorProgress(prev => ({
                downloaded: data.downloaded || 0,
                total: data.total || 0,
                albumID: data.albumID || '',
                currentFile: data.currentFile || '',
                phase: data.phase || '',
                startTime: (data.downloaded > 0 && !prev?.startTime) ? Date.now() : (prev?.startTime || 0),
            }));
        };

        const completeHandler = (data: any) => {
            if (!mountedRef.current) return;
            console.log('[cloud] download complete:', data);
            setMirroring(null);
            setMirrorProgress(null);
            if (data?.path) {
                onLoadDir(data.path);
                onClose();
            }
        };

        EventsOn('cloud-auth-complete', authHandler);
        EventsOn('cloud-download-progress', progressHandler);
        EventsOn('cloud-download-complete', completeHandler);

        return () => {
            EventsOff('cloud-auth-complete');
            EventsOff('cloud-download-progress');
            EventsOff('cloud-download-complete');
        };
    }, [onLoadDir, onClose]);

    const loadProviders = async () => {
        setLoadingProviders(true);
        setError(null);
        try {
            const sources = await GetCloudSources();
            if (!mountedRef.current) return;
            setProviders(sources || []);
        } catch (e) {
            console.error('[cloud] failed to load providers:', e);
            if (mountedRef.current) setError(`Failed to load cloud sources: ${e}`);
        } finally {
            if (mountedRef.current) setLoadingProviders(false);
        }
    };

    const loadAlbums = async (providerID: string) => {
        setLoadingAlbums(true);
        setAlbums([]);
        setError(null);
        try {
            const result = await ListCloudAlbums(providerID);
            if (!mountedRef.current) return;
            setAlbums(result || []);
        } catch (e) {
            console.error('[cloud] failed to load albums:', e);
            if (mountedRef.current) setError(`Failed to load albums: ${e}`);
        } finally {
            if (mountedRef.current) setLoadingAlbums(false);
        }
    };

    const handleConnect = async (providerID: string) => {
        setAuthenticating(providerID);
        setError(null);
        try {
            await AuthenticateCloudSource(providerID);
        } catch (e) {
            console.error('[cloud] auth failed:', e);
            if (mountedRef.current) {
                setAuthenticating(null);
                setError(`Authentication failed: ${e}`);
            }
        }
    };

    const handleDisconnect = async (providerID: string) => {
        setError(null);
        try {
            await DisconnectCloudSource(providerID);
            if (!mountedRef.current) return;
            if (selectedProvider === providerID) {
                setSelectedProvider(null);
                setAlbums([]);
            }
            loadProviders();
        } catch (e) {
            console.error('[cloud] disconnect failed:', e);
            if (mountedRef.current) setError(`Disconnect failed: ${e}`);
        }
    };

    const handleSelectProvider = (providerID: string) => {
        setSelectedProvider(providerID);
        loadAlbums(providerID);
    };

    const handleMirror = async (albumID: string, albumTitle: string) => {
        if (!selectedProvider) return;
        setMirroring(albumID);
        setMirrorProgress(null);
        setError(null);
        try {
            const localPath = await MirrorCloudAlbum(selectedProvider, albumID, albumTitle);
            if (!mountedRef.current) return;
            // If the backend returns the path directly (no event-based flow)
            if (localPath) {
                setMirroring(null);
                setMirrorProgress(null);
                onLoadDir(localPath);
                onClose();
            }
        } catch (e) {
            console.error('[cloud] mirror failed:', e);
            if (mountedRef.current) {
                setMirroring(null);
                setMirrorProgress(null);
                setError(`Mirror failed: ${e}`);
            }
        }
    };

    const handleCancelMirror = async () => {
        if (!selectedProvider || !mirroring) return;
        try {
            await CancelMirror(selectedProvider, mirroring);
        } catch (e) {
            console.error('[cloud] cancel mirror failed:', e);
        }
        if (mountedRef.current) {
            setMirroring(null);
            setMirrorProgress(null);
        }
    };

    const selectedProviderObj = providers.find(p => p.providerID === selectedProvider);

    return (
        <div className="settings-overlay" onClick={onClose}>
            <div className="settings-modal glass-panel" onClick={e => e.stopPropagation()} style={{ maxWidth: 560 }}>
                <div className="settings-header">
                    <h2 style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                        <Cloud size={20} />
                        Cloud Albums
                    </h2>
                    <button className="btn icon-btn" onClick={onClose}><X size={16} /></button>
                </div>

                {error && (
                    <div style={{
                        padding: '8px 12px',
                        marginBottom: 12,
                        background: 'rgba(239, 68, 68, 0.1)',
                        border: '1px solid rgba(239, 68, 68, 0.3)',
                        borderRadius: 8,
                        fontSize: '0.8rem',
                        color: '#f87171',
                    }}>
                        {error}
                    </div>
                )}

                {/* Mirror progress overlay */}
                {mirroring && (
                    <div style={{
                        padding: '20px',
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        gap: 12,
                    }}>
                        <Loader size={24} className="spin" style={{ color: 'var(--accent)' }} />
                        <div style={{ fontSize: '0.9rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                            {mirrorProgress && mirrorProgress.currentFile
                                ? `Exporting ${mirrorProgress.currentFile}`
                                : (mirrorProgress?.phase || 'Preparing export...')}
                        </div>
                        {mirrorProgress && mirrorProgress.total > 0 && (() => {
                            const pct = Math.min(100, (mirrorProgress.downloaded / mirrorProgress.total) * 100);
                            const elapsed = (Date.now() - mirrorProgress.startTime) / 1000;
                            const perFile = mirrorProgress.downloaded > 0 ? elapsed / mirrorProgress.downloaded : 0;
                            const remaining = Math.ceil(perFile * (mirrorProgress.total - mirrorProgress.downloaded));
                            const eta = mirrorProgress.downloaded > 0 && remaining > 0
                                ? remaining < 60 ? `~${remaining}s remaining` : `~${Math.ceil(remaining / 60)}m remaining`
                                : '';
                            return (
                                <>
                                    <div className="progress-bar-container-large">
                                        <div className="progress-bar-fill-large" style={{
                                            width: `${pct}%`,
                                            transition: 'width 0.3s ease-out',
                                        }} />
                                    </div>
                                    <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', textAlign: 'center' }}>
                                        {mirrorProgress.downloaded} / {mirrorProgress.total} files ({Math.round(pct)}%)
                                    </div>
                                    {eta && (
                                        <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', opacity: 0.7 }}>
                                            {eta}
                                        </div>
                                    )}
                                </>
                            );
                        })()}
                        <button
                            className="btn"
                            style={{ marginTop: 8, backgroundColor: 'var(--danger)', borderColor: 'var(--danger)' }}
                            onClick={handleCancelMirror}
                        >
                            Cancel
                        </button>
                    </div>
                )}

                {/* Provider list */}
                {!mirroring && (
                    <section className="settings-section">
                        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <h3>Sources</h3>
                            <button
                                className="btn icon-btn"
                                onClick={loadProviders}
                                title="Refresh providers"
                                style={{ padding: 4 }}
                            >
                                <RefreshCw size={14} />
                            </button>
                        </div>

                        {loadingProviders ? (
                            <div style={{ padding: '12px 0', textAlign: 'center', color: 'var(--text-secondary)', fontSize: '0.85rem' }}>
                                Loading sources...
                            </div>
                        ) : providers.length === 0 ? (
                            <div style={{ padding: '12px 0', textAlign: 'center', color: 'var(--text-muted)', fontSize: '0.85rem' }}>
                                No cloud sources available
                            </div>
                        ) : (
                            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                                {providers.map(provider => (
                                    <div
                                        key={provider.providerID}
                                        style={{
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'space-between',
                                            padding: '10px 12px',
                                            borderRadius: 8,
                                            background: selectedProvider === provider.providerID
                                                ? 'rgba(129, 140, 248, 0.15)'
                                                : 'rgba(255, 255, 255, 0.05)',
                                            border: selectedProvider === provider.providerID
                                                ? '1px solid rgba(129, 140, 248, 0.3)'
                                                : '1px solid transparent',
                                            cursor: provider.isConnected ? 'pointer' : 'default',
                                            transition: 'background 0.2s, border 0.2s',
                                        }}
                                        onClick={() => provider.isConnected && handleSelectProvider(provider.providerID)}
                                    >
                                        <div>
                                            <div style={{ fontWeight: 600, fontSize: '0.85rem', color: 'var(--text-primary)' }}>
                                                {provider.displayName}
                                            </div>
                                            {!provider.isAvailable && (
                                                <div style={{ fontSize: '0.72rem', color: 'var(--text-secondary)', marginTop: 2 }}>
                                                    Not available on this platform
                                                </div>
                                            )}
                                        </div>
                                        <div style={{ display: 'flex', gap: 6 }}>
                                            {provider.isConnected ? (
                                                <>
                                                    <button
                                                        className="btn"
                                                        style={{ fontSize: '0.75rem', padding: '4px 10px' }}
                                                        onClick={(e) => { e.stopPropagation(); handleSelectProvider(provider.providerID); }}
                                                    >
                                                        Browse
                                                    </button>
                                                    <button
                                                        className="btn"
                                                        style={{ fontSize: '0.75rem', padding: '4px 8px' }}
                                                        onClick={(e) => { e.stopPropagation(); handleDisconnect(provider.providerID); }}
                                                        title="Disconnect"
                                                    >
                                                        <LogOut size={12} />
                                                    </button>
                                                </>
                                            ) : (
                                                <button
                                                    className="btn btn-gradient"
                                                    style={{ fontSize: '0.75rem', padding: '4px 12px' }}
                                                    onClick={(e) => { e.stopPropagation(); handleConnect(provider.providerID); }}
                                                    disabled={authenticating === provider.providerID}
                                                >
                                                    {authenticating === provider.providerID ? 'Connecting...' : 'Connect'}
                                                </button>
                                            )}
                                        </div>
                                    </div>
                                ))}
                            </div>
                        )}
                    </section>
                )}

                {/* Album list */}
                {!mirroring && selectedProviderObj?.isConnected && (
                    <section className="settings-section">
                        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <h3>Albums — {selectedProviderObj.displayName}</h3>
                            <button
                                className="btn icon-btn"
                                onClick={() => selectedProvider && loadAlbums(selectedProvider)}
                                title="Refresh albums"
                                style={{ padding: 4 }}
                            >
                                <RefreshCw size={14} />
                            </button>
                        </div>

                        {loadingAlbums ? (
                            <div style={{ padding: '12px 0', textAlign: 'center', color: 'var(--text-secondary)', fontSize: '0.85rem' }}>
                                Loading albums...
                            </div>
                        ) : albums.length === 0 ? (
                            <div style={{ padding: '12px 0', textAlign: 'center', color: 'var(--text-muted)', fontSize: '0.85rem' }}>
                                No albums found
                            </div>
                        ) : (
                            <div style={{
                                display: 'flex',
                                flexDirection: 'column',
                                gap: 6,
                                maxHeight: 300,
                                overflowY: 'auto',
                            }}>
                                {albums.map(album => (
                                    <div
                                        key={album.id}
                                        style={{
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'space-between',
                                            padding: '8px 12px',
                                            borderRadius: 8,
                                            background: 'rgba(255, 255, 255, 0.05)',
                                        }}
                                    >
                                        <div>
                                            <div style={{ fontWeight: 500, fontSize: '0.85rem', color: 'var(--text-primary)' }}>
                                                {album.title}
                                            </div>
                                            <div style={{ fontSize: '0.72rem', color: 'var(--text-secondary)', marginTop: 2 }}>
                                                {album.mediaCount} item{album.mediaCount !== 1 ? 's' : ''}
                                            </div>
                                        </div>
                                        <button
                                            className="btn btn-gradient"
                                            style={{ fontSize: '0.75rem', padding: '4px 12px', display: 'flex', alignItems: 'center', gap: 4 }}
                                            onClick={() => handleMirror(album.id, album.title)}
                                        >
                                            <FolderDown size={12} />
                                            Mirror & Open
                                        </button>
                                    </div>
                                ))}
                            </div>
                        )}
                    </section>
                )}
            </div>
        </div>
    );
}
