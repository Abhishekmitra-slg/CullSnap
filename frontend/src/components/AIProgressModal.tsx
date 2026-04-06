import { useState, useEffect, useRef, useCallback } from 'react';
import { X, CheckCircle, Circle, Loader, AlertCircle } from 'lucide-react';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { CancelAIAnalysis } from '../../wailsjs/go/app/App';

type StepStatus = 'pending' | 'running' | 'done' | 'error';

interface Step {
    key: string;
    label: string;
    status: StepStatus;
    count?: number;
    total?: number;
    detail?: string;
}

interface CompleteSummary {
    scored: number;
    faces: number;
    clusters: number;
    topScore: number;
}

interface Props {
    visible: boolean;
    onClose: () => void;
    onComplete: () => void;
}

const INITIAL_STEPS: Step[] = [
    { key: 'scan', label: 'Scanning photos', status: 'pending' },
    { key: 'filter', label: 'Filtering candidates', status: 'pending' },
    { key: 'score', label: 'Scoring photos', status: 'pending' },
    { key: 'cluster', label: 'Clustering faces', status: 'pending' },
];

export function AIProgressModal({ visible, onClose, onComplete }: Props) {
    const [steps, setSteps] = useState<Step[]>(INITIAL_STEPS);
    const [errorMsg, setErrorMsg] = useState<string | null>(null);
    const [complete, setComplete] = useState(false);
    const [summary, setSummary] = useState<CompleteSummary | null>(null);
    const [elapsed, setElapsed] = useState(0);
    const [eta, setEta] = useState<string | null>(null);
    const startTimeRef = useRef<number>(0);
    const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
    const scoreStartRef = useRef<{ time: number; count: number } | null>(null);

    // Reset state when modal opens
    useEffect(() => {
        if (!visible) return;
        setSteps(INITIAL_STEPS);
        setErrorMsg(null);
        setComplete(false);
        setSummary(null);
        setElapsed(0);
        setEta(null);
        startTimeRef.current = Date.now();
        scoreStartRef.current = null;

        if (timerRef.current) clearInterval(timerRef.current);
        timerRef.current = setInterval(() => {
            setElapsed(Math.floor((Date.now() - startTimeRef.current) / 1000));
        }, 1000);

        return () => {
            if (timerRef.current) {
                clearInterval(timerRef.current);
                timerRef.current = null;
            }
        };
    }, [visible]);

    const updateStep = useCallback((key: string, patch: Partial<Step>) => {
        setSteps(prev => prev.map(s => s.key === key ? { ...s, ...patch } : s));
    }, []);

    useEffect(() => {
        if (!visible) return;

        const stepStarted = (data: any) => {
            console.log('[ai-modal] step-started:', data);
            const key = data?.step ?? '';
            // Map backend step names to our keys
            const mapped = mapStep(key);
            if (!mapped) return;
            setSteps(prev => prev.map(s =>
                s.key === mapped
                    ? { ...s, status: 'running', count: 0, total: data?.total ?? 0, detail: data?.detail }
                    : s
            ));
        };

        const stepProgress = (data: any) => {
            const key = mapStep(data?.step ?? '');
            if (!key) return;
            const count = data?.current ?? 0;
            const total = data?.total ?? 0;
            if (key === 'score') {
                if (!scoreStartRef.current) {
                    scoreStartRef.current = { time: Date.now(), count };
                } else if (count > scoreStartRef.current.count) {
                    const elapsed = (Date.now() - scoreStartRef.current.time) / 1000;
                    const rate = (count - scoreStartRef.current.count) / elapsed;
                    const remaining = total - count;
                    if (rate > 0 && remaining > 0) {
                        const secsLeft = remaining / rate;
                        if (secsLeft < 60) {
                            setEta(`~${Math.ceil(secsLeft)}s`);
                        } else {
                            const m = Math.floor(secsLeft / 60);
                            const s = Math.ceil(secsLeft % 60);
                            setEta(`~${m}m ${s}s`);
                        }
                    }
                }
            }
            updateStep(key, { count, total, status: 'running' });
        };

        const stepCompleted = (data: any) => {
            console.log('[ai-modal] step-completed:', data);
            const key = mapStep(data?.step ?? '');
            if (!key) return;
            updateStep(key, { status: 'done', count: data?.count, detail: data?.detail });
            if (key === 'score') setEta(null);
        };

        const errorHandler = (data: any) => {
            const msg = data?.error || data?.message || 'Unknown error';
            console.error('[ai-modal] error:', msg);
            setErrorMsg(msg);
            setSteps(prev => prev.map(s =>
                s.status === 'running' ? { ...s, status: 'error' } : s
            ));
            if (timerRef.current) {
                clearInterval(timerRef.current);
                timerRef.current = null;
            }
        };

        const completeHandler = (data: any) => {
            console.log('[ai-modal] complete:', data);
            if (timerRef.current) {
                clearInterval(timerRef.current);
                timerRef.current = null;
            }
            setEta(null);
            setSteps(prev => prev.map(s => s.status !== 'error' ? { ...s, status: 'done' } : s));
            setSummary({
                scored: data?.scored ?? 0,
                faces: data?.faces ?? 0,
                clusters: data?.clusters ?? 0,
                topScore: data?.topScore ?? 0,
            });
            setComplete(true);
        };

        const unsub1 = EventsOn('ai:step-started', stepStarted);
        const unsub2 = EventsOn('ai:step-progress', stepProgress);
        const unsub3 = EventsOn('ai:step-completed', stepCompleted);
        const unsub4 = EventsOn('ai:error', errorHandler);
        const unsub5 = EventsOn('ai:complete', completeHandler);

        return () => {
            unsub1();
            unsub2();
            unsub3();
            unsub4();
            unsub5();
        };
    }, [visible, updateStep]);

    const handleCancel = useCallback(() => {
        CancelAIAnalysis().catch((err: unknown) => console.warn('[ai-modal] cancel failed:', err));
    }, []);

    const handleViewResults = useCallback(() => {
        onComplete();
        onClose();
    }, [onComplete, onClose]);

    const formatElapsed = (s: number): string => {
        const m = Math.floor(s / 60);
        const sec = s % 60;
        return `${m}:${sec.toString().padStart(2, '0')}`;
    };

    if (!visible) return null;

    return (
        <div style={{
            position: 'fixed', top: 0, left: 0, right: 0, bottom: 0,
            zIndex: 9998,
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
                        AI Analysis
                    </h2>
                    <button
                        onClick={onClose}
                        style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#888', padding: 4 }}
                        title="Close"
                    >
                        <X size={16} />
                    </button>
                </div>

                {complete && summary ? (
                    /* Summary view */
                    <div>
                        <div style={{
                            background: 'rgba(74, 222, 128, 0.08)',
                            border: '1px solid rgba(74, 222, 128, 0.2)',
                            borderRadius: 8,
                            padding: '12px 16px',
                            marginBottom: 16,
                        }}>
                            <div style={{ color: '#4ade80', fontWeight: 600, fontSize: '0.85rem', marginBottom: 8 }}>
                                Analysis complete
                            </div>
                            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '6px 12px' }}>
                                <SummaryRow label="Photos analyzed" value={summary.scored} />
                                <SummaryRow label="Faces detected" value={summary.faces} />
                                <SummaryRow label="People clusters" value={summary.clusters} />
                                <SummaryRow label="Top score" value={summary.topScore} />
                            </div>
                        </div>
                        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                            <button
                                className="btn"
                                style={{ fontSize: '0.8rem', padding: '6px 14px' }}
                                onClick={onClose}
                            >
                                Close
                            </button>
                            <button
                                className="btn btn-primary"
                                style={{ fontSize: '0.8rem', padding: '6px 14px' }}
                                onClick={handleViewResults}
                            >
                                View Results
                            </button>
                        </div>
                    </div>
                ) : (
                    /* Progress view */
                    <div>
                        {errorMsg && (
                            <div style={{
                                background: 'rgba(239, 68, 68, 0.1)',
                                border: '1px solid rgba(239, 68, 68, 0.3)',
                                borderRadius: 6,
                                padding: '8px 12px',
                                marginBottom: 16,
                                display: 'flex', alignItems: 'flex-start', gap: 8,
                                fontSize: '0.78rem', color: '#ef4444',
                            }}>
                                <AlertCircle size={14} style={{ flexShrink: 0, marginTop: 1 }} />
                                {errorMsg}
                            </div>
                        )}

                        {/* Steps */}
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 10, marginBottom: 20 }}>
                            {steps.map(step => (
                                <StepRow key={step.key} step={step} />
                            ))}
                        </div>

                        {/* Footer */}
                        <div style={{
                            display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                            borderTop: '1px solid #2a2a3e', paddingTop: 14,
                        }}>
                            <span style={{ color: '#666', fontSize: '0.72rem' }}>
                                {formatElapsed(elapsed)}{eta ? ` · ${eta} remaining` : ''}
                            </span>
                            {!errorMsg ? (
                                <button
                                    onClick={handleCancel}
                                    style={{
                                        background: 'none', border: '1px solid #444',
                                        borderRadius: 6, padding: '4px 12px',
                                        color: '#ef4444', fontSize: '0.78rem', cursor: 'pointer',
                                    }}
                                >
                                    Cancel
                                </button>
                            ) : (
                                <button
                                    onClick={onClose}
                                    style={{
                                        background: 'none', border: '1px solid #444',
                                        borderRadius: 6, padding: '4px 12px',
                                        color: '#ccc', fontSize: '0.78rem', cursor: 'pointer',
                                    }}
                                >
                                    Close
                                </button>
                            )}
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}

function StepRow({ step }: { step: Step }) {
    const pct = step.total && step.total > 0
        ? Math.min(100, Math.round((step.count ?? 0) / step.total * 100))
        : 0;

    return (
        <div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <StepIcon status={step.status} />
                <span style={{
                    flex: 1, fontSize: '0.82rem',
                    color: step.status === 'done' ? '#888' : step.status === 'error' ? '#ef4444' : '#ccc',
                }}>
                    {step.label}
                </span>
                {step.status === 'running' && step.total != null && step.total > 0 && (
                    <span style={{ fontSize: '0.7rem', color: '#666' }}>
                        {step.count ?? 0} / {step.total}
                    </span>
                )}
                {step.status === 'done' && step.count != null && step.total == null && (
                    <span style={{ fontSize: '0.7rem', color: '#666' }}>{step.count}</span>
                )}
            </div>
            {step.status === 'running' && step.total != null && step.total > 0 && (
                <div style={{
                    marginLeft: 24, marginTop: 4,
                    background: '#2a2a3e', borderRadius: 2, height: 3, overflow: 'hidden',
                }}>
                    <div style={{
                        height: '100%',
                        width: `${pct}%`,
                        background: '#818cf8',
                        transition: 'width 200ms ease-out',
                        borderRadius: 2,
                    }} />
                </div>
            )}
        </div>
    );
}

function StepIcon({ status }: { status: StepStatus }) {
    switch (status) {
        case 'done':
            return <CheckCircle size={16} color="#4ade80" />;
        case 'running':
            return (
                <Loader
                    size={16}
                    color="#818cf8"
                    style={{ animation: 'spin 1s linear infinite' }}
                />
            );
        case 'error':
            return <AlertCircle size={16} color="#ef4444" />;
        default:
            return <Circle size={16} color="#444" />;
    }
}

function SummaryRow({ label, value }: { label: string; value: number }) {
    return (
        <>
            <span style={{ fontSize: '0.75rem', color: '#888' }}>{label}</span>
            <span style={{ fontSize: '0.75rem', color: '#ccc', fontWeight: 600 }}>{value}</span>
        </>
    );
}

function mapStep(step: string): string | null {
    const m: Record<string, string> = {
        scan: 'scan',
        scanning: 'scan',
        filter: 'filter',
        filtering: 'filter',
        score: 'score',
        scoring: 'score',
        cluster: 'cluster',
        clustering: 'cluster',
    };
    return m[step.toLowerCase()] ?? null;
}
