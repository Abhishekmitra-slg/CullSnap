import { useState, useEffect } from 'react';
import { X, Download, RotateCcw, AlertTriangle, Loader } from 'lucide-react';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import { DownloadUpdate, RestartForUpdate } from '../../wailsjs/go/app/App';

type ToastState =
    | { kind: 'available'; version: string; releaseURL: string; homebrew: boolean }
    | { kind: 'downloading'; version: string }
    | { kind: 'ready' }
    | { kind: 'error'; message: string }
    | null;

export function UpdateToast() {
    const [toast, setToast] = useState<ToastState>(null);
    const [dismissed, setDismissed] = useState(false);

    useEffect(() => {
        EventsOn('update:available', (data: { version: string; releaseURL: string; homebrew: boolean }) => {
            setToast({ kind: 'available', version: data.version, releaseURL: data.releaseURL, homebrew: data.homebrew });
            setDismissed(false);
        });

        EventsOn('update:downloading', (data: { version: string }) => {
            setToast({ kind: 'downloading', version: data.version });
            setDismissed(false);
        });

        EventsOn('update:ready', () => {
            setToast({ kind: 'ready' });
            setDismissed(false);
        });

        EventsOn('update:error', (data: { message: string }) => {
            setToast({ kind: 'error', message: data.message });
            setDismissed(false);
        });

        return () => {
            EventsOff('update:available');
            EventsOff('update:downloading');
            EventsOff('update:ready');
            EventsOff('update:error');
        };
    }, []);

    if (!toast || dismissed) return null;

    const borderColor =
        toast.kind === 'error' ? '#ef4444' :
        toast.kind === 'downloading' ? '#3b82f6' :
        '#22c55e';

    return (
        <div
            style={{
                position: 'fixed',
                bottom: 40,
                right: 16,
                zIndex: 1000,
                background: 'rgb(24, 24, 27)',
                border: `1px solid ${borderColor}`,
                borderRadius: 10,
                padding: '14px 16px',
                width: 300,
                boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
                color: 'var(--text-primary)',
            }}
        >
            <button
                onClick={() => setDismissed(true)}
                style={{
                    position: 'absolute',
                    top: 8,
                    right: 8,
                    background: 'none',
                    border: 'none',
                    cursor: 'pointer',
                    color: 'var(--text-secondary)',
                    display: 'flex',
                    alignItems: 'center',
                    padding: 2,
                }}
            >
                <X size={14} />
            </button>

            {toast.kind === 'available' && !toast.homebrew && (
                <>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                        <Download size={15} color="#22c55e" />
                        <span style={{ fontWeight: 600, fontSize: '0.85rem' }}>Update Available</span>
                    </div>
                    <p style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', margin: '0 0 12px' }}>
                        Version {toast.version} is ready to download.
                    </p>
                    <div style={{ display: 'flex', gap: 8 }}>
                        <button
                            className="btn btn-primary"
                            style={{ fontSize: '0.75rem', padding: '5px 12px', flex: 1 }}
                            onClick={() => DownloadUpdate().catch(console.error)}
                        >
                            Download &amp; Install
                        </button>
                        <button
                            className="btn outline"
                            style={{ fontSize: '0.75rem', padding: '5px 12px' }}
                            onClick={() => setDismissed(true)}
                        >
                            Later
                        </button>
                    </div>
                </>
            )}

            {toast.kind === 'available' && toast.homebrew && (
                <>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                        <Download size={15} color="#22c55e" />
                        <span style={{ fontWeight: 600, fontSize: '0.85rem' }}>Update Available</span>
                    </div>
                    <p style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', margin: '0 0 8px' }}>
                        Version {toast.version} is available via Homebrew.
                    </p>
                    <code style={{
                        display: 'block',
                        fontSize: '0.75rem',
                        background: 'rgba(255,255,255,0.07)',
                        borderRadius: 5,
                        padding: '5px 8px',
                        color: 'var(--text-primary)',
                        marginBottom: 10,
                    }}>
                        brew upgrade cullsnap
                    </code>
                    <button
                        className="btn outline"
                        style={{ fontSize: '0.75rem', padding: '5px 12px' }}
                        onClick={() => setDismissed(true)}
                    >
                        Dismiss
                    </button>
                </>
            )}

            {toast.kind === 'downloading' && (
                <>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                        <Loader size={15} color="#3b82f6" className="spin" />
                        <span style={{ fontWeight: 600, fontSize: '0.85rem' }}>Downloading Update</span>
                    </div>
                    <p style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', margin: 0 }}>
                        Downloading version {toast.version}…
                    </p>
                </>
            )}

            {toast.kind === 'ready' && (
                <>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                        <RotateCcw size={15} color="#22c55e" />
                        <span style={{ fontWeight: 600, fontSize: '0.85rem' }}>Update Ready</span>
                    </div>
                    <p style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', margin: '0 0 12px' }}>
                        Applied to disk. Restart to use the new version.
                    </p>
                    <div style={{ display: 'flex', gap: 8 }}>
                        <button
                            className="btn btn-primary"
                            style={{ fontSize: '0.75rem', padding: '5px 12px', flex: 1 }}
                            onClick={() => RestartForUpdate().catch(console.error)}
                        >
                            Restart Now
                        </button>
                        <button
                            className="btn outline"
                            style={{ fontSize: '0.75rem', padding: '5px 12px' }}
                            onClick={() => setDismissed(true)}
                        >
                            Later
                        </button>
                    </div>
                </>
            )}

            {toast.kind === 'error' && (
                <>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                        <AlertTriangle size={15} color="#ef4444" />
                        <span style={{ fontWeight: 600, fontSize: '0.85rem' }}>Update Failed</span>
                    </div>
                    <p style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', margin: 0 }}>
                        {toast.message}
                    </p>
                </>
            )}
        </div>
    );
}
