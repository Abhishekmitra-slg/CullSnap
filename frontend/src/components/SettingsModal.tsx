import { useState, useEffect } from 'react';
import { X, RotateCcw, Cloud, Trash2, Smartphone } from 'lucide-react';
import { GetAppConfig, SaveAppConfig, ResetAppConfig, GetCacheStats, ListCachedAlbums, DeleteCachedAlbum, ClearAllCache, GetMirrorStats, ClearCloudMirror, GetImportStats, ClearImportCache } from '../../wailsjs/go/app/App';
import { app } from '../../wailsjs/go/models';

interface SettingsModalProps {
    onClose: () => void;
}

export function SettingsModal({ onClose }: SettingsModalProps) {
    const [config, setConfig] = useState<app.AppConfig | null>(null);
    const [saving, setSaving] = useState(false);
    const [mirrorStats, setMirrorStats] = useState<any>(null);
    const [clearingMirrors, setClearingMirrors] = useState(false);
    const [importStats, setImportStats] = useState<any>(null);
    const [clearingImport, setClearingImport] = useState<string | null>(null);
    const [cachedAlbums, setCachedAlbums] = useState<any[]>([]);
    const [deletingAlbum, setDeletingAlbum] = useState<string | null>(null);

    useEffect(() => {
        GetAppConfig().then(setConfig).catch(console.error);
        loadMirrorStats();
        loadImportStats();
    }, []);

    const loadMirrorStats = async () => {
        try {
            const stats = await GetCacheStats();
            setMirrorStats(stats);
        } catch (e) {
            console.error('[settings] failed to load cache stats:', e);
        }
        try {
            const albums = await ListCachedAlbums();
            setCachedAlbums(albums || []);
        } catch (e) {
            console.error('[settings] failed to load cached albums:', e);
        }
    };

    const [confirmAction, setConfirmAction] = useState<{ message: string; action: () => void } | null>(null);

    const handleClearAllMirrors = () => {
        setConfirmAction({
            message: 'Clear all cached cloud albums? Files can be re-mirrored later.',
            action: async () => {
                setConfirmAction(null);
                setClearingMirrors(true);
                try {
                    await ClearAllCache();
                } catch (e) {
                    console.error('[settings] failed to clear cache:', e);
                }
                await loadMirrorStats();
                setClearingMirrors(false);
            },
        });
    };

    const handleDeleteAlbum = (providerID: string, albumID: string, title: string) => {
        setConfirmAction({
            message: `Remove cached files for "${title}"? You can re-mirror later.`,
            action: async () => {
                setConfirmAction(null);
                setDeletingAlbum(albumID);
                try {
                    await DeleteCachedAlbum(providerID, albumID);
                } catch (e) {
                    console.error('[settings] failed to delete album:', e);
                }
                await loadMirrorStats();
                setDeletingAlbum(null);
            },
        });
    };

    const relativeTime = (dateStr: string): string => {
        const now = Date.now();
        const then = new Date(dateStr).getTime();
        if (isNaN(then)) return 'unknown';
        const diffSec = Math.floor((now - then) / 1000);
        if (diffSec < 60) return 'just now';
        if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
        if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
        const days = Math.floor(diffSec / 86400);
        if (days === 1) return '1 day ago';
        if (days < 30) return `${days} days ago`;
        return `${Math.floor(days / 30)}mo ago`;
    };

    const loadImportStats = async () => {
        try {
            const stats = await GetImportStats();
            setImportStats(stats);
        } catch (e) {
            console.error('[settings] failed to load import stats:', e);
        }
    };

    const handleClearImportCache = async (serial: string) => {
        if (!window.confirm('Clear cached imports for this device? This will delete locally stored files.')) return;
        setClearingImport(serial);
        try {
            await ClearImportCache(serial);
            await loadImportStats();
        } catch (e) {
            console.error('[settings] failed to clear import cache:', e);
        } finally {
            setClearingImport(null);
        }
    };

    const formatBytes = (bytes: number): string => {
        if (!bytes || bytes === 0) return '0 B';
        const units = ['B', 'KB', 'MB', 'GB'];
        const i = Math.floor(Math.log(bytes) / Math.log(1024));
        return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
    };

    const handleSave = async () => {
        if (!config) return;
        setSaving(true);
        try {
            await SaveAppConfig(config);
        } catch (e) {
            console.error('Failed to save config:', e);
        } finally {
            setSaving(false);
        }
    };

    const handleReset = async () => {
        try {
            const newConfig = await ResetAppConfig();
            setConfig(newConfig);
        } catch (e) {
            console.error('Failed to reset config:', e);
        }
    };

    if (!config) {
        return (
            <div className="settings-overlay">
                <div className="settings-modal glass-panel">
                    <p style={{ color: 'var(--text-secondary)' }}>Loading...</p>
                </div>
            </div>
        );
    }

    return (
        <div className="settings-overlay" onClick={onClose}>
            <div className="settings-modal glass-panel" onClick={e => e.stopPropagation()}>
                <div className="settings-header">
                    <h2>Settings</h2>
                    <button className="btn icon-btn" onClick={onClose}><X size={16} /></button>
                </div>

                <section className="settings-section">
                    <h3>System Info</h3>
                    <div className="settings-info-grid">
                        <span>OS</span><span>{config.probe.OS} / {config.probe.Arch}</span>
                        <span>CPU Cores</span><span>{config.probe.CPUs}</span>
                        <span>RAM</span><span>{config.probe.RAMMB} MB</span>
                        <span>FFmpeg</span><span>{config.probe.FFmpegReady ? '✓ Available' : '✗ Not found'}</span>
                        <span>Storage</span><span>{config.probe.StorageHint}</span>
                    </div>
                </section>

                <section className="settings-section">
                    <h3>Performance Tuning</h3>
                    <label className="settings-label">
                        Max Connections
                        <span className="settings-hint">(media server concurrent requests, 10–50)</span>
                        <input type="range" min={10} max={50} value={config.maxConnections}
                            onChange={e => setConfig(app.AppConfig.createFrom({ ...config, maxConnections: +e.target.value }))} />
                        <span>{config.maxConnections}</span>
                    </label>
                    <label className="settings-label">
                        Thumbnail Workers
                        <span className="settings-hint">(parallel thumbnail generation, 2–8)</span>
                        <input type="range" min={2} max={8} value={config.thumbnailWorkers}
                            onChange={e => setConfig(app.AppConfig.createFrom({ ...config, thumbnailWorkers: +e.target.value }))} />
                        <span>{config.thumbnailWorkers}</span>
                    </label>
                    <label className="settings-label">
                        Scanner Workers
                        <span className="settings-hint">(parallel video duration probing, 1–4)</span>
                        <input type="range" min={1} max={4} value={config.scannerWorkers}
                            onChange={e => setConfig(app.AppConfig.createFrom({ ...config, scannerWorkers: +e.target.value }))} />
                        <span>{config.scannerWorkers}</span>
                    </label>
                </section>

                <section className="settings-section">
                    <h3>Updates</h3>
                    <label className="settings-label">
                        Auto-Update
                        <span className="settings-hint">(how CullSnap handles new versions)</span>
                        <select
                            value={config.autoUpdate}
                            onChange={e => setConfig(app.AppConfig.createFrom({ ...config, autoUpdate: e.target.value }))}
                            style={{
                                background: 'rgba(255,255,255,0.1)',
                                border: '1px solid rgba(255,255,255,0.2)',
                                borderRadius: 6,
                                padding: '6px 12px',
                                color: 'white',
                                fontSize: '0.85rem',
                                width: '100%',
                                marginTop: 4,
                            }}
                        >
                            <option value="off">Off</option>
                            <option value="notify">Notify Only</option>
                            <option value="auto">Auto-Update</option>
                        </select>
                    </label>
                    <div style={{ fontSize: '0.7rem', color: 'var(--text-secondary)', lineHeight: 1.6, marginTop: 8, background: 'rgba(0,0,0,0.2)', borderRadius: 6, padding: '8px 10px' }}>
                        <div><strong>Off</strong> — No update checks, no network calls</div>
                        <div><strong>Notify Only</strong> — Checks for updates, notifies when available</div>
                        <div><strong>Auto-Update</strong> — Downloads and applies updates automatically</div>
                    </div>
                    <div style={{ fontSize: '0.7rem', color: 'var(--text-secondary)', marginTop: 6, fontStyle: 'italic' }}>
                        Changes take effect after restart.
                    </div>
                </section>

                {config.probe?.OS === 'darwin' && (
                    <section className="settings-section">
                        <h3>HEIC Decoder</h3>
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                            <div>
                                <div style={{ fontWeight: 600, fontSize: '0.85rem' }}>Use native macOS HEIC decoder</div>
                                <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: 2 }}>
                                    Uses sips for fast HEIC thumbnail generation
                                </div>
                            </div>
                            <label style={{ position: 'relative', display: 'inline-block', width: 40, height: 22, flexShrink: 0 }}>
                                <input
                                    type="checkbox"
                                    checked={config.useNativeSips}
                                    onChange={(e) => {
                                        if (!e.target.checked) {
                                            if (!window.confirm(
                                                'Are you sure? FFmpeg HEIC decoding is 3-5x slower than the native macOS decoder. ' +
                                                'Thumbnail generation for HEIC photos will take significantly longer.'
                                            )) {
                                                return;
                                            }
                                        }
                                        setConfig(app.AppConfig.createFrom({ ...config, useNativeSips: e.target.checked }));
                                    }}
                                    style={{ opacity: 0, width: 0, height: 0 }}
                                />
                                <span style={{
                                    position: 'absolute', cursor: 'pointer', top: 0, left: 0, right: 0, bottom: 0,
                                    backgroundColor: config.useNativeSips ? '#818cf8' : '#555',
                                    borderRadius: 11, transition: 'background-color 0.2s',
                                }} />
                                <span style={{
                                    position: 'absolute', content: '""', height: 18, width: 18,
                                    left: config.useNativeSips ? 20 : 2, top: 2,
                                    backgroundColor: config.useNativeSips ? 'white' : '#999',
                                    borderRadius: '50%', transition: 'left 0.2s, background-color 0.2s',
                                }} />
                            </label>
                        </div>
                        {!config.useNativeSips && (
                            <div style={{
                                marginTop: 12, padding: '8px 10px',
                                background: 'rgba(255, 180, 50, 0.08)',
                                border: '1px solid rgba(255, 180, 50, 0.2)',
                                borderRadius: 8, fontSize: '0.75rem', color: '#d4a017',
                            }}>
                                <strong>&#9888; Slower decoding active</strong> — HEIC photos will be decoded using FFmpeg,
                                which is 3-5x slower than the native macOS decoder.
                            </div>
                        )}
                    </section>
                )}

                <section className="settings-section">
                    <h3 style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                        <Cloud size={14} />
                        Cloud Storage
                    </h3>

                    {/* Inline confirmation dialog (window.confirm doesn't work in WKWebView) */}
                    {confirmAction && (
                        <div style={{
                            background: 'var(--bg-panel)',
                            border: '1px solid var(--danger)',
                            borderRadius: 6,
                            padding: '12px 16px',
                            marginBottom: 12,
                            fontSize: '0.85rem',
                        }}>
                            <div style={{ marginBottom: 10, color: 'var(--text-primary)' }}>{confirmAction.message}</div>
                            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
                                <button className="btn outline" style={{ fontSize: '0.8rem', padding: '4px 12px' }}
                                    onClick={() => setConfirmAction(null)}>
                                    Cancel
                                </button>
                                <button className="btn" style={{ fontSize: '0.8rem', padding: '4px 12px', backgroundColor: 'var(--danger)', borderColor: 'var(--danger)' }}
                                    onClick={confirmAction.action}>
                                    Confirm
                                </button>
                            </div>
                        </div>
                    )}

                    {/* Usage bar */}
                    {mirrorStats && (
                        <div style={{ marginBottom: 12 }}>
                            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.8rem', marginBottom: 4 }}>
                                <span>{formatBytes(mirrorStats.totalBytes || 0)} of {formatBytes(mirrorStats.limitBytes || 0)} used</span>
                                <span>{mirrorStats.albumCount || 0} album{(mirrorStats.albumCount || 0) !== 1 ? 's' : ''}</span>
                            </div>
                            <div className="progress-bar-container-large" style={{ width: '100%', maxWidth: 'none' }}>
                                <div className="progress-bar-fill-large" style={{
                                    width: `${Math.min(100, ((mirrorStats.totalBytes || 0) / (mirrorStats.limitBytes || 1)) * 100)}%`,
                                    background: ((mirrorStats.totalBytes || 0) / (mirrorStats.limitBytes || 1)) > 0.8
                                        ? 'var(--danger)' : 'var(--accent-gradient)',
                                    transition: 'width 0.3s ease',
                                }} />
                            </div>
                        </div>
                    )}

                    {/* Cache limit slider */}
                    {config && (
                        <div className="settings-info-grid" style={{ marginBottom: 12 }}>
                            <span>Cache Limit</span>
                            <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                                <input
                                    type="range"
                                    min={1}
                                    max={50}
                                    value={Math.round((config.maxCloudCacheMB || 10240) / 1024)}
                                    onChange={(e) => {
                                        const gb = parseInt(e.target.value);
                                        const updated = app.AppConfig.createFrom({ ...config, maxCloudCacheMB: gb * 1024 });
                                        setConfig(updated);
                                        SaveAppConfig(updated).catch(console.error);
                                    }}
                                    style={{ flex: 1 }}
                                />
                                <span style={{ minWidth: 40, textAlign: 'right', fontSize: '0.8rem' }}>
                                    {Math.round((config.maxCloudCacheMB || 10240) / 1024)} GB
                                </span>
                            </span>
                        </div>
                    )}

                    {/* Cached albums list */}
                    {cachedAlbums.length > 0 ? (
                        <div style={{
                            maxHeight: 200,
                            overflowY: 'auto',
                            border: '1px solid var(--border)',
                            borderRadius: 6,
                            marginBottom: 10,
                        }}>
                            {cachedAlbums.map((album: any) => (
                                <div key={`${album.providerID}-${album.albumID}`} style={{
                                    display: 'flex',
                                    alignItems: 'center',
                                    justifyContent: 'space-between',
                                    padding: '8px 12px',
                                    borderBottom: '1px solid var(--border)',
                                    fontSize: '0.8rem',
                                }}>
                                    <div style={{ flex: 1, minWidth: 0 }}>
                                        <div style={{ fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                            {album.albumTitle}
                                        </div>
                                        <div style={{ color: 'var(--text-secondary)', fontSize: '0.75rem' }}>
                                            {album.providerID === 'icloud' ? 'iCloud Photos' : 'Google Drive'}
                                            {' \u00B7 '}
                                            {formatBytes(album.sizeBytes || 0)}, {album.fileCount || 0} files
                                            {' \u00B7 Synced '}
                                            {album.syncedAt ? relativeTime(album.syncedAt) : 'unknown'}
                                        </div>
                                    </div>
                                    <button
                                        className="btn icon-btn"
                                        style={{ marginLeft: 8, padding: 4 }}
                                        onClick={() => handleDeleteAlbum(album.providerID, album.albumID, album.albumTitle)}
                                        disabled={deletingAlbum === album.albumID}
                                        title="Remove cached files"
                                    >
                                        <Trash2 size={14} />
                                    </button>
                                </div>
                            ))}
                        </div>
                    ) : (
                        <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', padding: '12px 0', textAlign: 'center' }}>
                            No cached albums
                        </div>
                    )}

                    <button
                        className="btn outline"
                        style={{ fontSize: '0.8rem' }}
                        onClick={handleClearAllMirrors}
                        disabled={clearingMirrors || (cachedAlbums.length === 0 && (!mirrorStats || mirrorStats.albumCount === 0))}
                    >
                        <Trash2 size={12} />
                        {clearingMirrors ? 'Clearing...' : 'Clear All Cache'}
                    </button>
                </section>

                {config.probe?.OS === 'darwin' && (
                    <section className="settings-section">
                        <h3 style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                            <Smartphone size={14} />
                            Device Import Cache
                        </h3>
                        <div className="settings-info-grid">
                            <span>Total Disk Usage</span>
                            <span>{importStats ? formatBytes(importStats.totalBytes || 0) : 'Loading...'}</span>
                        </div>
                        {importStats && importStats.deviceStats && Object.keys(importStats.deviceStats).length > 0 && (
                            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginTop: 8 }}>
                                {Object.entries(importStats.deviceStats as Record<string, number>).map(([serial, bytes]) => (
                                    <div key={serial} style={{
                                        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                                        padding: '6px 10px', borderRadius: 6,
                                        background: 'rgba(255,255,255,0.03)',
                                    }}>
                                        <div>
                                            <div style={{ fontSize: '0.8rem', color: 'var(--text-primary)' }}>
                                                {serial.substring(0, 12)}...
                                            </div>
                                            <div style={{ fontSize: '0.7rem', color: 'var(--text-secondary)' }}>
                                                {formatBytes(bytes)}
                                            </div>
                                        </div>
                                        <button
                                            className="btn"
                                            style={{ fontSize: '0.72rem', padding: '3px 8px' }}
                                            onClick={() => handleClearImportCache(serial)}
                                            disabled={clearingImport === serial}
                                        >
                                            <Trash2 size={10} />
                                            {clearingImport === serial ? 'Clearing...' : 'Clear'}
                                        </button>
                                    </div>
                                ))}
                            </div>
                        )}
                        {importStats && (importStats.totalBytes || 0) === 0 && (
                            <div style={{ fontSize: '0.75rem', color: 'var(--text-muted)', marginTop: 6 }}>
                                No device imports cached
                            </div>
                        )}
                    </section>
                )}

                <div className="settings-footer">
                    <button className="btn outline" onClick={handleReset}>
                        <RotateCcw size={14} /> Reset to Defaults
                    </button>
                    <button className="btn btn-primary" onClick={handleSave} disabled={saving}>
                        {saving ? 'Saving...' : 'Save'}
                    </button>
                </div>
            </div>
        </div>
    );
}
