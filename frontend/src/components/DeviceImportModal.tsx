import { useState, useEffect, useRef } from 'react';
import { X, Smartphone, RefreshCw, Loader, FolderOpen, AlertTriangle, Camera, HardDrive, Copy, Terminal } from 'lucide-react';
import { GetConnectedDevices, ImportFromDevice, CheckDeviceDependencies } from '../../wailsjs/go/app/App';
import { EventsOn, EventsOff, ClipboardSetText } from '../../wailsjs/runtime/runtime';

interface DeviceImportModalProps {
    onClose: () => void;
    onLoadDir: (dir: string) => void;
    probe?: { OS: string };
}

interface DeviceInfo {
    name: string;
    vendorID: string;
    productID: string;
    serial: string;
    type: string;
    mountPath: string;
    detectedAt: string;
}

interface DependencyStatusInfo {
    usbmuxdRunning: boolean;
    gvfsAvailable: boolean;
    gphoto2Path: string;
    ideviceInfoPath: string;
    distroID: string;
    distroFamily: string;
    distroName: string;
    installCommand: string;
    missingPackages: string[];
}

const allInstallCommands = [
    { label: 'Debian/Ubuntu', cmd: 'sudo apt install libimobiledevice-utils usbmuxd gphoto2' },
    { label: 'Fedora/RHEL', cmd: 'sudo dnf install libimobiledevice-utils usbmuxd gphoto2' },
    { label: 'Arch', cmd: 'sudo pacman -S libimobiledevice usbmuxd gphoto2' },
    { label: 'openSUSE', cmd: 'sudo zypper install libimobiledevice-utils usbmuxd gphoto2' },
];

function deviceIcon(type: string) {
    switch (type) {
        case 'camera': return <Camera size={18} style={{ color: 'var(--accent)', flexShrink: 0 }} />;
        case 'storage': return <HardDrive size={18} style={{ color: 'var(--accent)', flexShrink: 0 }} />;
        default: return <Smartphone size={18} style={{ color: 'var(--accent)', flexShrink: 0 }} />;
    }
}

export function DeviceImportModal({ onClose, onLoadDir, probe }: DeviceImportModalProps) {
    const [devices, setDevices] = useState<DeviceInfo[]>([]);
    const [loading, setLoading] = useState(true);
    const [importing, setImporting] = useState<string | null>(null);
    const [importResult, setImportResult] = useState<{ serial: string; count: number; path: string } | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [depStatus, setDepStatus] = useState<DependencyStatusInfo | null>(null);
    const [depChecking, setDepChecking] = useState(false);
    const [copied, setCopied] = useState(false);
    const mountedRef = useRef(true);

    useEffect(() => {
        mountedRef.current = true;
        return () => { mountedRef.current = false; };
    }, []);

    // Load devices on mount
    useEffect(() => {
        loadDevices();
    }, []);

    // Check dependencies on Linux
    useEffect(() => {
        if (probe?.OS === 'linux') {
            checkDeps();
        }
    }, [probe?.OS]);

    // Listen for device events
    useEffect(() => {
        const connectHandler = () => {
            if (!mountedRef.current) return;
            console.log('[device] device connected, refreshing list');
            loadDevices();
        };

        const disconnectHandler = () => {
            if (!mountedRef.current) return;
            console.log('[device] device disconnected, refreshing list');
            loadDevices();
        };

        const completeHandler = (data: any) => {
            if (!mountedRef.current) return;
            console.log('[device] import complete:', data);
            setImporting(null);
            setImportResult({
                serial: data?.serial || '',
                count: data?.count || 0,
                path: data?.path || '',
            });
        };

        const errorHandler = (data: any) => {
            if (!mountedRef.current) return;
            console.log('[device] import error:', data);
            setImporting(null);
            const errMsg = data?.error || 'Import failed';
            setError(errMsg);
            if (data?.count > 0 && data?.path) {
                setImportResult({
                    serial: data?.serial || '',
                    count: data.count,
                    path: data.path,
                });
            }
        };

        EventsOn('device-connected', connectHandler);
        EventsOn('device-disconnected', disconnectHandler);
        EventsOn('device-import-complete', completeHandler);
        EventsOn('device-import-error', errorHandler);

        return () => {
            EventsOff('device-connected');
            EventsOff('device-disconnected');
            EventsOff('device-import-complete');
            EventsOff('device-import-error');
        };
    }, []);

    const loadDevices = async () => {
        setLoading(true);
        setError(null);
        try {
            const result = await GetConnectedDevices();
            if (!mountedRef.current) return;
            setDevices(result || []);
        } catch (e) {
            console.error('[device] failed to load devices:', e);
            if (mountedRef.current) setError(`Failed to detect devices: ${e}`);
        } finally {
            if (mountedRef.current) setLoading(false);
        }
    };

    const checkDeps = async () => {
        setDepChecking(true);
        try {
            const status = await CheckDeviceDependencies();
            if (mountedRef.current) setDepStatus(status);
        } catch (e) {
            console.error('[device] dep check failed:', e);
        } finally {
            if (mountedRef.current) setDepChecking(false);
        }
    };

    const handleCopyCommand = async (cmd?: string) => {
        const text = cmd || depStatus?.installCommand;
        if (text) {
            await ClipboardSetText(text);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        }
    };

    const handleImport = async (serial: string) => {
        setImporting(serial);
        setError(null);
        setImportResult(null);
        try {
            await ImportFromDevice(serial);
        } catch (e) {
            console.error('[device] import call failed:', e);
            if (mountedRef.current) {
                setImporting(null);
                setError(`Import failed: ${e}`);
            }
        }
    };

    const handleOpenImported = () => {
        if (importResult?.path) {
            onLoadDir(importResult.path);
            onClose();
        }
    };

    const showDepGuide = probe?.OS === 'linux' && depStatus && depStatus.missingPackages && depStatus.missingPackages.length > 0;

    return (
        <div className="settings-overlay" onClick={onClose}>
            <div className="settings-modal glass-panel" onClick={e => e.stopPropagation()} style={{ maxWidth: 520 }}>
                <div className="settings-header">
                    <h2 style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                        <Smartphone size={20} />
                        Import from Device
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
                        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 6 }}>
                            <AlertTriangle size={14} style={{ flexShrink: 0, marginTop: 1 }} />
                            <div>{error}</div>
                        </div>
                    </div>
                )}

                {/* Import progress overlay */}
                {importing && (
                    <div style={{
                        padding: '20px',
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        gap: 12,
                    }}>
                        <Loader size={24} className="spin" style={{ color: 'var(--accent)' }} />
                        <div style={{ fontSize: '0.9rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                            Importing photos...
                        </div>
                        <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', textAlign: 'center' }}>
                            {probe?.OS === 'darwin'
                                ? <>Image Capture will open to import photos from your device.<br />This may take a moment.</>
                                : <>Copying photos from your device.<br />This may take a moment.</>
                            }
                        </div>
                    </div>
                )}

                {/* Import complete state */}
                {!importing && importResult && (
                    <div style={{
                        padding: '20px',
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        gap: 12,
                    }}>
                        <div style={{
                            width: 48, height: 48, borderRadius: '50%',
                            background: 'rgba(34, 197, 94, 0.15)',
                            display: 'flex', alignItems: 'center', justifyContent: 'center',
                        }}>
                            <FolderOpen size={24} style={{ color: '#22c55e' }} />
                        </div>
                        <div style={{ fontSize: '0.9rem', fontWeight: 600, color: 'var(--text-primary)' }}>
                            {importResult.count} file{importResult.count !== 1 ? 's' : ''} imported
                        </div>
                        <button
                            className="btn btn-gradient"
                            style={{ padding: '8px 20px' }}
                            onClick={handleOpenImported}
                        >
                            <FolderOpen size={14} />
                            Open in CullSnap
                        </button>
                    </div>
                )}

                {/* Linux dependency setup guide */}
                {!importing && !importResult && showDepGuide && (
                    <section className="settings-section">
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                            <Terminal size={16} style={{ color: 'var(--accent)' }} />
                            <h3 style={{ margin: 0 }}>Device Import Setup</h3>
                        </div>
                        <p style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', margin: '0 0 12px 0' }}>
                            CullSnap needs a few system packages to communicate with your device.
                        </p>

                        {depStatus!.installCommand ? (
                            <div style={{
                                display: 'flex',
                                alignItems: 'center',
                                gap: 8,
                                padding: '10px 12px',
                                background: 'rgba(0, 0, 0, 0.2)',
                                borderRadius: 8,
                                fontFamily: 'monospace',
                                fontSize: '0.75rem',
                                color: 'var(--text-primary)',
                                marginBottom: 8,
                            }}>
                                <code style={{ flex: 1, overflowX: 'auto', whiteSpace: 'nowrap' }}>
                                    {depStatus!.installCommand}
                                </code>
                                <button
                                    className="btn icon-btn"
                                    onClick={() => handleCopyCommand()}
                                    title="Copy to clipboard"
                                    style={{ padding: 4, flexShrink: 0 }}
                                >
                                    <Copy size={14} style={{ color: copied ? '#22c55e' : 'var(--text-secondary)' }} />
                                </button>
                            </div>
                        ) : (
                            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginBottom: 8 }}>
                                {allInstallCommands.map(({ label, cmd }) => (
                                    <div key={label} style={{
                                        padding: '8px 10px',
                                        background: 'rgba(0, 0, 0, 0.2)',
                                        borderRadius: 6,
                                        fontSize: '0.72rem',
                                        display: 'flex',
                                        alignItems: 'center',
                                        gap: 6,
                                    }}>
                                        <div style={{ flex: 1 }}>
                                            <div style={{ color: 'var(--text-secondary)', marginBottom: 2 }}>{label}:</div>
                                            <code style={{ color: 'var(--text-primary)', fontFamily: 'monospace' }}>{cmd}</code>
                                        </div>
                                        <button
                                            className="btn icon-btn"
                                            onClick={() => handleCopyCommand(cmd)}
                                            title="Copy"
                                            style={{ padding: 3, flexShrink: 0 }}
                                        >
                                            <Copy size={12} style={{ color: 'var(--text-secondary)' }} />
                                        </button>
                                    </div>
                                ))}
                            </div>
                        )}

                        {depStatus!.distroName && (
                            <div style={{ fontSize: '0.72rem', color: 'var(--text-muted)', marginBottom: 8 }}>
                                Detected: {depStatus!.distroName}
                            </div>
                        )}

                        <button
                            className="btn w-full"
                            onClick={checkDeps}
                            disabled={depChecking}
                            style={{ marginTop: 4 }}
                        >
                            <RefreshCw size={14} className={depChecking ? 'spin' : ''} />
                            {depChecking ? 'Checking...' : 'Check Again'}
                        </button>
                    </section>
                )}

                {/* Device list */}
                {!importing && !importResult && (
                    <section className="settings-section">
                        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <h3>Connected Devices</h3>
                            <button
                                className="btn icon-btn"
                                onClick={loadDevices}
                                title="Refresh devices"
                                style={{ padding: 4 }}
                            >
                                <RefreshCw size={14} />
                            </button>
                        </div>

                        {loading ? (
                            <div style={{ padding: '12px 0', textAlign: 'center', color: 'var(--text-secondary)', fontSize: '0.85rem' }}>
                                Scanning for devices...
                            </div>
                        ) : devices.length === 0 ? (
                            <div style={{ padding: '16px 0', textAlign: 'center' }}>
                                <Smartphone size={32} style={{ color: 'var(--text-muted)', marginBottom: 8 }} />
                                <div style={{ color: 'var(--text-muted)', fontSize: '0.85rem', marginBottom: 8 }}>
                                    No devices detected
                                </div>
                                <div style={{ color: 'var(--text-secondary)', fontSize: '0.72rem', lineHeight: 1.6 }}>
                                    Connect your device via USB and unlock it.
                                    <br />
                                    If prompted, tap "Trust This Computer" on your device.
                                    {probe?.OS === 'linux' && (
                                        <>
                                            <br />
                                            For Android, select "File Transfer" mode when prompted.
                                        </>
                                    )}
                                </div>
                            </div>
                        ) : (
                            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                                {devices.map(dev => (
                                    <div
                                        key={dev.serial}
                                        style={{
                                            display: 'flex',
                                            alignItems: 'center',
                                            justifyContent: 'space-between',
                                            padding: '10px 12px',
                                            borderRadius: 8,
                                            background: 'rgba(255, 255, 255, 0.05)',
                                            border: '1px solid transparent',
                                        }}
                                    >
                                        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                                            {deviceIcon(dev.type || '')}
                                            <div>
                                                <div style={{ fontWeight: 600, fontSize: '0.85rem', color: 'var(--text-primary)' }}>
                                                    {dev.name}
                                                </div>
                                                <div style={{ fontSize: '0.68rem', color: 'var(--text-secondary)', marginTop: 2 }}>
                                                    {dev.serial.substring(0, 12)}{dev.serial.length > 12 ? '...' : ''}
                                                </div>
                                            </div>
                                        </div>
                                        <button
                                            className="btn btn-gradient"
                                            style={{ fontSize: '0.75rem', padding: '6px 14px' }}
                                            onClick={() => handleImport(dev.serial)}
                                        >
                                            Import Photos
                                        </button>
                                    </div>
                                ))}
                            </div>
                        )}
                    </section>
                )}

                {/* Manual fallback instructions */}
                {!importing && !importResult && (
                    <div style={{
                        padding: '10px 12px',
                        background: 'rgba(255, 255, 255, 0.03)',
                        borderRadius: 8,
                        fontSize: '0.72rem',
                        color: 'var(--text-secondary)',
                        lineHeight: 1.6,
                    }}>
                        <strong>Manual import:</strong>{' '}
                        {probe?.OS === 'darwin'
                            ? 'Open Image Capture from Spotlight, select your device, drag photos to a folder, then open that folder in CullSnap.'
                            : 'Connect your device, find its DCIM folder in your file manager, copy photos to a folder, then open that folder in CullSnap.'}
                    </div>
                )}
            </div>
        </div>
    );
}
