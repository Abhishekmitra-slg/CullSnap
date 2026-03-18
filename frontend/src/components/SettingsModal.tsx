import { useState, useEffect } from 'react';
import { X, RotateCcw } from 'lucide-react';
import { GetAppConfig, SaveAppConfig, ResetAppConfig } from '../../wailsjs/go/app/App';
import { app } from '../../wailsjs/go/models';

interface SettingsModalProps {
    onClose: () => void;
}

export function SettingsModal({ onClose }: SettingsModalProps) {
    const [config, setConfig] = useState<app.AppConfig | null>(null);
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        GetAppConfig().then(setConfig).catch(console.error);
    }, []);

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
