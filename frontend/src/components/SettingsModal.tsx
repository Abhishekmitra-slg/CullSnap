import { useState, useEffect } from 'react';
import { X, RotateCcw, Cloud, Trash2 } from 'lucide-react';
import { GetAppConfig, SaveAppConfig, ResetAppConfig, GetMirrorStats, ClearCloudMirror } from '../../wailsjs/go/app/App';
import { app } from '../../wailsjs/go/models';

interface SettingsModalProps {
    onClose: () => void;
}

export function SettingsModal({ onClose }: SettingsModalProps) {
    const [config, setConfig] = useState<app.AppConfig | null>(null);
    const [saving, setSaving] = useState(false);
    const [mirrorStats, setMirrorStats] = useState<any>(null);
    const [clearingMirrors, setClearingMirrors] = useState(false);

    useEffect(() => {
        GetAppConfig().then(setConfig).catch(console.error);
        loadMirrorStats();
    }, []);

    const loadMirrorStats = async () => {
        try {
            const stats = await GetMirrorStats();
            setMirrorStats(stats);
        } catch (e) {
            console.error('[settings] failed to load mirror stats:', e);
        }
    };

    const handleClearAllMirrors = async () => {
        if (!window.confirm('Clear all mirrored cloud albums? This will delete locally cached files.')) return;
        setClearingMirrors(true);
        try {
            await ClearCloudMirror('', '');
            await loadMirrorStats();
        } catch (e) {
            console.error('[settings] failed to clear mirrors:', e);
        } finally {
            setClearingMirrors(false);
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
                    <div className="settings-info-grid">
                        <span>Mirror Disk Usage</span>
                        <span>{mirrorStats ? formatBytes(mirrorStats.totalBytes || 0) : 'Loading...'}</span>
                        <span>Cached Albums</span>
                        <span>{mirrorStats ? (mirrorStats.albumCount || 0) : '...'}</span>
                    </div>
                    <button
                        className="btn outline"
                        style={{ marginTop: 10, fontSize: '0.8rem' }}
                        onClick={handleClearAllMirrors}
                        disabled={clearingMirrors || !mirrorStats || (mirrorStats.totalBytes || 0) === 0}
                    >
                        <Trash2 size={12} />
                        {clearingMirrors ? 'Clearing...' : 'Clear All Mirrors'}
                    </button>
                </section>

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
