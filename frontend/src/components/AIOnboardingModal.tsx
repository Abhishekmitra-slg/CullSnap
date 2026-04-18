import { useState, useEffect, useRef, useCallback } from 'react';
import { X, CheckCircle, AlertCircle, Loader, Download } from 'lucide-react';
import { EventsOn } from '../../wailsjs/runtime/runtime';

interface Props {
    visible: boolean;
    onClose: () => void;
}

interface VLMStatus {
    state: string;
    modelName: string;
    backend: string;
    uptime: number;
    available: boolean;
    hardwareTier: string;
}

interface DownloadProgress {
    stage: 'model' | 'runtime';
    downloaded?: number;
    total?: number;
    model?: string;
    runtime?: string;
}

interface ModelOption {
    id: string;
    label: string;
    sizeMB: string;
    description: string;
    minTier: string;
}

const MODEL_OPTIONS: ModelOption[] = [
    {
        id: 'gemma-4-e4b-it',
        label: 'Gemma 4 E4B',
        sizeMB: '2.8 GB',
        description: 'Best quality',
        minTier: 'capable',
    },
    {
        id: 'gemma-4-e2b-it',
        label: 'Gemma 4 E2B',
        sizeMB: '1.5 GB',
        description: 'Faster, lighter',
        minTier: 'basic',
    },
];

const TIER_RANK: Record<string, number> = {
    power: 3,
    capable: 2,
    basic: 1,
    unsupported: 0,
};

function recommendedModel(tier: string): string {
    const rank = TIER_RANK[tier] ?? 0;
    if (rank >= TIER_RANK['capable']) {
        return 'gemma-4-e4b-it';
    }
    return 'gemma-4-e2b-it';
}

function tierMeetsMin(tier: string, minTier: string): boolean {
    return (TIER_RANK[tier] ?? 0) >= (TIER_RANK[minTier] ?? 0);
}

function formatBytes(bytes: number): string {
    if (bytes <= 0) return '0 B';
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

export function AIOnboardingModal({ visible, onClose }: Props) {
    const [status, setStatus] = useState<VLMStatus | null>(null);
    const [statusLoading, setStatusLoading] = useState(false);
    const [statusError, setStatusError] = useState<string | null>(null);
    const [selectedModel, setSelectedModel] = useState<string>('');
    const [downloading, setDownloading] = useState(false);
    const [downloadError, setDownloadError] = useState<string | null>(null);
    const [complete, setComplete] = useState(false);
    const [progress, setProgress] = useState<DownloadProgress | null>(null);
    const unsubRef = useRef<(() => void) | null>(null);

    // Fetch hardware status on open
    useEffect(() => {
        if (!visible) return;

        // Reset modal state whenever it opens
        setStatus(null);
        setStatusError(null);
        setDownloading(false);
        setDownloadError(null);
        setComplete(false);
        setProgress(null);

        setStatusLoading(true);
        console.log('[vlm-onboarding] fetching VLM status');
        import('../../wailsjs/go/app/App').then((mod) => {
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            const GetVLMStatus = (mod as any).GetVLMStatus as () => Promise<VLMStatus>;
            return GetVLMStatus();
        }).then((s: VLMStatus) => {
            console.log('[vlm-onboarding] status:', s);
            setStatus(s);
            if (s.available) {
                setSelectedModel(recommendedModel(s.hardwareTier));
            }
        }).catch((err: unknown) => {
            const msg = err instanceof Error ? err.message : String(err);
            console.error('[vlm-onboarding] status error:', msg);
            setStatusError(msg);
        }).finally(() => {
            setStatusLoading(false);
        });
    }, [visible]);

    // Subscribe to download progress events
    useEffect(() => {
        if (!visible || !downloading) return;

        const unsub = EventsOn('vlm:download-progress', (data: DownloadProgress) => {
            console.log('[vlm-onboarding] download progress:', data);
            setProgress(data);
        });
        unsubRef.current = unsub;

        return () => {
            unsub();
            unsubRef.current = null;
        };
    }, [visible, downloading]);

    const handleDownload = useCallback(() => {
        if (!selectedModel) return;
        setDownloading(true);
        setDownloadError(null);
        setProgress(null);
        console.log('[vlm-onboarding] starting download:', selectedModel);

        import('../../wailsjs/go/app/App').then((mod) => {
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            const DownloadVLMModel = (mod as any).DownloadVLMModel as (modelName: string) => Promise<void>;
            return DownloadVLMModel(selectedModel);
        }).then(() => {
            console.log('[vlm-onboarding] download complete');
            setComplete(true);
            setDownloading(false);
            if (unsubRef.current) {
                unsubRef.current();
                unsubRef.current = null;
            }
        }).catch((err: unknown) => {
            const msg = err instanceof Error ? err.message : String(err);
            console.error('[vlm-onboarding] download error:', msg);
            setDownloadError(msg);
            setDownloading(false);
        });
    }, [selectedModel]);

    if (!visible) return null;

    return (
        <div style={{
            position: 'fixed', top: 0, left: 0, right: 0, bottom: 0,
            zIndex: 9999,
            background: 'rgba(0, 0, 0, 0.75)',
            backdropFilter: 'blur(4px)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}>
            <div style={{
                background: '#1e1e2e',
                border: '1px solid #333',
                borderRadius: 12,
                padding: 28,
                width: 420,
                maxWidth: '90vw',
                boxShadow: '0 24px 64px rgba(0,0,0,0.6)',
            }}>
                {/* Header */}
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 20 }}>
                    <h2 style={{ margin: 0, fontSize: '1rem', fontWeight: 600, color: '#e0e0e0' }}>
                        Set up AI Description
                    </h2>
                    <button
                        onClick={onClose}
                        disabled={downloading}
                        style={{
                            background: 'none', border: 'none', cursor: downloading ? 'not-allowed' : 'pointer',
                            color: '#888', padding: 4, opacity: downloading ? 0.4 : 1,
                        }}
                        title="Close"
                    >
                        <X size={16} />
                    </button>
                </div>

                {/* Body */}
                {statusLoading && (
                    <div style={{ display: 'flex', alignItems: 'center', gap: 10, color: '#888', fontSize: '0.85rem', padding: '20px 0' }}>
                        <Loader size={16} color="#818cf8" style={{ animation: 'spin 1s linear infinite' }} />
                        Checking hardware compatibility...
                    </div>
                )}

                {!statusLoading && statusError && (
                    <div style={{
                        background: 'rgba(239, 68, 68, 0.1)',
                        border: '1px solid rgba(239, 68, 68, 0.3)',
                        borderRadius: 8,
                        padding: '12px 14px',
                        display: 'flex', alignItems: 'flex-start', gap: 8,
                        fontSize: '0.8rem', color: '#ef4444',
                        marginBottom: 16,
                    }}>
                        <AlertCircle size={14} style={{ flexShrink: 0, marginTop: 1 }} />
                        {statusError}
                    </div>
                )}

                {!statusLoading && status && !status.available && (
                    <div style={{
                        background: 'rgba(239, 68, 68, 0.08)',
                        border: '1px solid rgba(239, 68, 68, 0.25)',
                        borderRadius: 8,
                        padding: '14px 16px',
                        fontSize: '0.83rem',
                        color: '#ef4444',
                        lineHeight: 1.5,
                    }}>
                        VLM not supported on this hardware.
                    </div>
                )}

                {!statusLoading && status && status.available && !complete && (
                    <>
                        {/* Model selection */}
                        <div style={{ marginBottom: 20 }}>
                            <div style={{ fontSize: '0.75rem', color: '#888', marginBottom: 10, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                                Select model
                            </div>
                            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                                {MODEL_OPTIONS.map(model => {
                                    const isSelected = selectedModel === model.id;
                                    const isRecommended = recommendedModel(status.hardwareTier) === model.id;
                                    const isCompatible = tierMeetsMin(status.hardwareTier, model.minTier);

                                    return (
                                        <button
                                            key={model.id}
                                            onClick={() => { if (isCompatible && !downloading) setSelectedModel(model.id); }}
                                            disabled={!isCompatible || downloading}
                                            style={{
                                                background: isSelected ? 'rgba(129, 140, 248, 0.12)' : 'rgba(255,255,255,0.03)',
                                                border: isSelected ? '1px solid rgba(129, 140, 248, 0.5)' : '1px solid #2e2e42',
                                                borderRadius: 8,
                                                padding: '10px 14px',
                                                cursor: isCompatible && !downloading ? 'pointer' : 'not-allowed',
                                                opacity: !isCompatible ? 0.45 : 1,
                                                display: 'flex',
                                                alignItems: 'center',
                                                justifyContent: 'space-between',
                                                textAlign: 'left',
                                                width: '100%',
                                                transition: 'border-color 120ms, background 120ms',
                                            }}
                                        >
                                            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                                                {/* Radio indicator */}
                                                <div style={{
                                                    width: 16, height: 16, borderRadius: '50%',
                                                    border: isSelected ? '2px solid #818cf8' : '2px solid #444',
                                                    background: isSelected ? '#818cf8' : 'transparent',
                                                    flexShrink: 0,
                                                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                                                    transition: 'border-color 120ms, background 120ms',
                                                }}>
                                                    {isSelected && (
                                                        <div style={{ width: 6, height: 6, borderRadius: '50%', background: '#fff' }} />
                                                    )}
                                                </div>
                                                <div>
                                                    <div style={{ fontSize: '0.85rem', fontWeight: 600, color: '#e0e0e0', display: 'flex', alignItems: 'center', gap: 6 }}>
                                                        {model.label}
                                                        {isRecommended && (
                                                            <span style={{
                                                                fontSize: '0.62rem',
                                                                fontWeight: 700,
                                                                color: '#818cf8',
                                                                background: 'rgba(129, 140, 248, 0.15)',
                                                                border: '1px solid rgba(129, 140, 248, 0.3)',
                                                                borderRadius: 4,
                                                                padding: '1px 5px',
                                                                letterSpacing: '0.04em',
                                                                textTransform: 'uppercase',
                                                            }}>
                                                                RECOMMENDED
                                                            </span>
                                                        )}
                                                    </div>
                                                    <div style={{ fontSize: '0.73rem', color: '#888', marginTop: 2 }}>
                                                        {model.description}
                                                    </div>
                                                </div>
                                            </div>
                                            <div style={{ fontSize: '0.75rem', color: '#666', flexShrink: 0, marginLeft: 8 }}>
                                                {model.sizeMB}
                                            </div>
                                        </button>
                                    );
                                })}
                            </div>
                        </div>

                        {/* Download progress */}
                        {downloading && progress && (
                            <div style={{ marginBottom: 16 }}>
                                <div style={{ fontSize: '0.75rem', color: '#888', marginBottom: 6 }}>
                                    {progress.stage === 'model'
                                        ? `Downloading model${progress.model ? ` (${progress.model})` : ''}`
                                        : `Downloading runtime${progress.runtime ? ` (${progress.runtime})` : ''}`}
                                </div>
                                {progress.total != null && progress.total > 0 ? (
                                    <>
                                        <div style={{
                                            background: '#2a2a3e', borderRadius: 4, height: 6, overflow: 'hidden',
                                        }}>
                                            <div style={{
                                                height: '100%',
                                                width: `${Math.min(100, ((progress.downloaded ?? 0) / progress.total) * 100).toFixed(1)}%`,
                                                background: '#818cf8',
                                                transition: 'width 200ms ease-out',
                                                borderRadius: 4,
                                            }} />
                                        </div>
                                        <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 4, fontSize: '0.7rem', color: '#666' }}>
                                            <span>{formatBytes(progress.downloaded ?? 0)}</span>
                                            <span>{Math.min(100, Math.round(((progress.downloaded ?? 0) / progress.total) * 100))}%</span>
                                            <span>{formatBytes(progress.total)}</span>
                                        </div>
                                    </>
                                ) : (
                                    <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: '#818cf8', fontSize: '0.78rem' }}>
                                        <Loader size={13} style={{ animation: 'spin 1s linear infinite' }} />
                                        Downloading...
                                    </div>
                                )}
                            </div>
                        )}

                        {/* Downloading spinner (no progress yet) */}
                        {downloading && !progress && (
                            <div style={{ display: 'flex', alignItems: 'center', gap: 8, color: '#818cf8', fontSize: '0.78rem', marginBottom: 16 }}>
                                <Loader size={13} style={{ animation: 'spin 1s linear infinite' }} />
                                Starting download...
                            </div>
                        )}

                        {/* Error */}
                        {downloadError && (
                            <div style={{
                                background: 'rgba(239, 68, 68, 0.1)',
                                border: '1px solid rgba(239, 68, 68, 0.3)',
                                borderRadius: 6,
                                padding: '8px 12px',
                                display: 'flex', alignItems: 'flex-start', gap: 8,
                                fontSize: '0.78rem', color: '#ef4444',
                                marginBottom: 16,
                            }}>
                                <AlertCircle size={13} style={{ flexShrink: 0, marginTop: 1 }} />
                                {downloadError}
                            </div>
                        )}

                        {/* Actions */}
                        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                            <button
                                className="btn"
                                style={{ fontSize: '0.8rem', padding: '6px 14px' }}
                                onClick={onClose}
                                disabled={downloading}
                            >
                                Cancel
                            </button>
                            <button
                                className="btn btn-primary"
                                style={{
                                    fontSize: '0.8rem', padding: '6px 14px',
                                    display: 'flex', alignItems: 'center', gap: 6,
                                    opacity: (!selectedModel || downloading) ? 0.6 : 1,
                                    cursor: (!selectedModel || downloading) ? 'not-allowed' : 'pointer',
                                }}
                                onClick={handleDownload}
                                disabled={!selectedModel || downloading}
                            >
                                {downloading ? (
                                    <>
                                        <Loader size={13} style={{ animation: 'spin 1s linear infinite' }} />
                                        Downloading...
                                    </>
                                ) : (
                                    <>
                                        <Download size={13} />
                                        Download
                                    </>
                                )}
                            </button>
                        </div>
                    </>
                )}

                {/* Completion state */}
                {complete && (
                    <div>
                        <div style={{
                            background: 'rgba(74, 222, 128, 0.08)',
                            border: '1px solid rgba(74, 222, 128, 0.2)',
                            borderRadius: 8,
                            padding: '14px 16px',
                            display: 'flex', alignItems: 'center', gap: 10,
                            marginBottom: 20,
                        }}>
                            <CheckCircle size={18} color="#4ade80" style={{ flexShrink: 0 }} />
                            <div>
                                <div style={{ color: '#4ade80', fontWeight: 600, fontSize: '0.88rem' }}>
                                    Setup complete!
                                </div>
                                <div style={{ color: '#888', fontSize: '0.75rem', marginTop: 2 }}>
                                    The model is ready. AI descriptions will be available on next analysis.
                                </div>
                            </div>
                        </div>
                        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                            <button
                                className="btn btn-primary"
                                style={{ fontSize: '0.8rem', padding: '6px 16px' }}
                                onClick={onClose}
                            >
                                Done
                            </button>
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}
