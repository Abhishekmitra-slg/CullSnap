import { useState, useRef, useEffect, useCallback } from 'react';
import {
    X, ChevronDown, ChevronRight, Sparkles, Users, Eye, EyeOff,
} from 'lucide-react';
import {
    GetPhotoAIScore,
    RenameFaceCluster,
    MergeFaceClusters,
    HideFaceCluster,
    CancelAIAnalysis,
} from '../../wailsjs/go/app/App';
import './AIPanel.css';

type TabId = 'scores' | 'rankings' | 'faces';

interface AIProgressData {
    scored: number;
    total: number;
    phase: string;
}

interface AIClusterData {
    id: number;
    label: string;
    count: number;
    thumbnail: string;
}

interface Props {
    visible: boolean;
    onClose: () => void;
    activePhotoPath: string;
    analysisProgress: AIProgressData | null;
    isAnalyzing: boolean;
    clusters: AIClusterData[];
    aiScores: Record<string, { score: number; faceCount: number }>;
    onFilterByFace: (clusterIds: number[]) => void;
    onSortByScore: (enabled: boolean) => void;
    onMinQuality: (threshold: number) => void;
    onHasFacesFilter: (enabled: boolean) => void;
    providerName: string;
    providerReady: boolean;
    onOpenSettings: () => void;
    onReanalyze?: () => void;
}

function scoreColor(score: number): string {
    if (score >= 80) return 'green';
    if (score >= 50) return 'yellow';
    return 'red';
}

function SubScoreBar({ label, value, color }: { label: string; value: number; color: string }) {
    const pct = Math.round(Math.min(1, Math.max(0, value)) * 100);
    return (
        <div className="ai-score-bar-container" style={{ marginBottom: 4 }}>
            <span className="ai-score-label" style={{ minWidth: 80 }}>{label}</span>
            <div style={{ display: 'flex', alignItems: 'center', gap: 4, flex: 1 }}>
                <div className="ai-score-bar" style={{ flex: 1 }}>
                    <div
                        className="ai-score-bar-fill"
                        style={{ width: `${pct}%`, background: color }}
                    />
                </div>
                <span className="ai-score-value" style={{ color, minWidth: 28, textAlign: 'right' }}>{pct}</span>
            </div>
        </div>
    );
}

export default function AIPanel({
    visible, onClose, activePhotoPath, analysisProgress, isAnalyzing,
    clusters, aiScores, onFilterByFace, onSortByScore, onMinQuality,
    onHasFacesFilter, providerName, providerReady, onOpenSettings, onReanalyze,
}: Props) {
    // Section collapse state
    const [statusOpen, setStatusOpen] = useState(true);
    const [filterOpen, setFilterOpen] = useState(false);

    // Tab state
    const [activeTab, setActiveTab] = useState<TabId>('scores');

    // Face chip state
    const [activeChips, setActiveChips] = useState<Set<number>>(new Set());
    const [editingLabel, setEditingLabel] = useState<number | null>(null);
    const [editValue, setEditValue] = useState('');
    const [mergeMode, setMergeMode] = useState<number | null>(null);
    const [contextMenu, setContextMenu] = useState<{ x: number; y: number; clusterId: number } | null>(null);

    // Selected photo AI data
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const [photoScore, setPhotoScore] = useState<any>(null);

    // VLM score for active photo
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const [vlmScore, setVlmScore] = useState<any>(null);

    // Rankings section
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const [rankings, setRankings] = useState<any[]>([]);

    // Stale VLM indicator
    const [staleVLM, setStaleVLM] = useState(false);

    // Filter state
    const [sortByScore, setSortByScore] = useState(false);
    const [minQuality, setMinQuality] = useState(0);
    const [hasFaces, setHasFaces] = useState(false);

    // Elapsed time for analysis
    const [elapsed, setElapsed] = useState(0);
    const elapsedRef = useRef<ReturnType<typeof setInterval> | null>(null);

    // Track analysis start for elapsed timer
    useEffect(() => {
        if (isAnalyzing) {
            setElapsed(0);
            elapsedRef.current = setInterval(() => setElapsed(prev => prev + 1), 1000);
        } else if (elapsedRef.current) {
            clearInterval(elapsedRef.current);
            elapsedRef.current = null;
        }
        return () => { if (elapsedRef.current) clearInterval(elapsedRef.current); };
    }, [isAnalyzing]);

    // Auto-expand status section during analysis
    useEffect(() => {
        if (isAnalyzing) setStatusOpen(true);
    }, [isAnalyzing]);

    // Load photo AI score when active photo changes
    useEffect(() => {
        if (!activePhotoPath) {
            setPhotoScore(null);
            return;
        }
        GetPhotoAIScore(activePhotoPath)
            .then((result: unknown) => setPhotoScore(result))
            .catch((err: unknown) => {
                console.warn('Failed to get photo AI score:', err);
                setPhotoScore(null);
            });
    }, [activePhotoPath]);

    // Load VLM score when active photo changes
    useEffect(() => {
        if (!activePhotoPath) {
            setVlmScore(null);
            return;
        }
        console.debug('[AIPanel] fetching VLM score for', activePhotoPath);
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        import('../../wailsjs/go/app/App').then((mod: any) => {
            return mod.GetVLMScoresForPhoto(activePhotoPath) as Promise<unknown>;
        }).then((result: unknown) => {
            console.debug('[AIPanel] VLM score result:', result);
            setVlmScore(result);
        }).catch((err: unknown) => {
            console.warn('[AIPanel] Failed to get VLM score:', err);
            setVlmScore(null);
        });
    }, [activePhotoPath]);

    // Load rankings and stale VLM status when panel becomes visible and we have scored photos
    useEffect(() => {
        if (!visible) return;
        const paths = Object.keys(aiScores);
        const folderPath = paths.length > 0 && paths[0].includes('/')
            ? paths[0].substring(0, paths[0].lastIndexOf('/'))
            : '';

        if (folderPath) {
            console.debug('[AIPanel] fetching VLM rankings for folder', folderPath);
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            import('../../wailsjs/go/app/App').then((mod: any) => {
                return mod.GetVLMRankingsForFolder(folderPath) as Promise<unknown>;
            }).then((result: unknown) => {
                console.debug('[AIPanel] VLM rankings result:', result);
                // eslint-disable-next-line @typescript-eslint/no-explicit-any
                setRankings(Array.isArray(result) ? (result as any[]) : []);
            }).catch((err: unknown) => {
                console.warn('[AIPanel] Failed to get VLM rankings:', err);
                setRankings([]);
            });
        }

        console.debug('[AIPanel] checking stale VLM status');
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        import('../../wailsjs/go/app/App').then((mod: any) => {
            return mod.GetStaleVLMStatus() as Promise<unknown>;
        }).then((result: unknown) => {
            console.debug('[AIPanel] stale VLM status:', result);
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            const status = result as any;
            setStaleVLM(typeof status?.stale === 'boolean' ? status.stale : false);
        }).catch((err: unknown) => {
            console.warn('[AIPanel] Failed to get stale VLM status:', err);
            setStaleVLM(false);
        });
    }, [visible, aiScores]);

    // Close context menu on click outside
    useEffect(() => {
        if (!contextMenu) return;
        const handler = () => setContextMenu(null);
        window.addEventListener('click', handler);
        return () => window.removeEventListener('click', handler);
    }, [contextMenu]);

    // Close context menu and merge mode on Escape
    useEffect(() => {
        const handler = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                setContextMenu(null);
                setMergeMode(null);
                setEditingLabel(null);
            }
        };
        window.addEventListener('keydown', handler);
        return () => window.removeEventListener('keydown', handler);
    }, []);

    const handleChipClick = useCallback((clusterId: number) => {
        if (mergeMode !== null) {
            // In merge mode: merge clicked chip into merge source
            if (clusterId !== mergeMode) {
                MergeFaceClusters(mergeMode, clusterId).catch((err: unknown) =>
                    console.warn('Merge failed:', err)
                );
            }
            setMergeMode(null);
            return;
        }

        setActiveChips(prev => {
            const next = new Set(prev);
            if (next.has(clusterId)) {
                next.delete(clusterId);
            } else {
                next.add(clusterId);
            }
            onFilterByFace(Array.from(next));
            return next;
        });
    }, [mergeMode, onFilterByFace]);

    const handleRename = useCallback((clusterId: number, newLabel: string) => {
        if (newLabel.trim()) {
            RenameFaceCluster(clusterId, newLabel.trim()).catch((err: unknown) =>
                console.warn('Rename failed:', err)
            );
        }
        setEditingLabel(null);
    }, []);

    const handleContextMenu = useCallback((e: React.MouseEvent, clusterId: number) => {
        e.preventDefault();
        setContextMenu({ x: e.clientX, y: e.clientY, clusterId });
    }, []);

    const handleCancel = useCallback(() => {
        CancelAIAnalysis().catch((err: unknown) => console.warn('Cancel failed:', err));
    }, []);

    const formatElapsed = (seconds: number): string => {
        const m = Math.floor(seconds / 60);
        const s = seconds % 60;
        return `${m}:${s.toString().padStart(2, '0')}`;
    };

    const activeFilterCount = [sortByScore, minQuality > 0, hasFaces].filter(Boolean).length
        + activeChips.size;

    const photoData = photoScore?.score;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const photoDetections: any[] = photoScore?.detections || [];

    // Suppress unused import warnings — icons used in JSX below
    void Sparkles; void Users; void Eye; void EyeOff;

    return (
        <div className={`ai-panel ${visible ? '' : 'collapsed'}`}>
            {/* Header */}
            <div className="ai-panel-header">
                <h3>AI Analysis</h3>
                <button className="btn icon-btn" onClick={onClose} title="Close AI Panel">
                    <X size={14} />
                </button>
            </div>

            <div className="ai-panel-content">
                {/* Analysis Status — always visible above tabs */}
                <div className="ai-section">
                    <div
                        className="ai-section-header"
                        onClick={() => setStatusOpen(!statusOpen)}
                        tabIndex={0}
                        onKeyDown={e => e.key === 'Enter' && setStatusOpen(!statusOpen)}
                        role="button"
                        aria-expanded={statusOpen}
                    >
                        <span className="ai-section-title">Status</span>
                        {statusOpen ? <ChevronDown size={14} color="#8b8ba7" /> : <ChevronRight size={14} color="#8b8ba7" />}
                    </div>
                    <div className="ai-section-body" style={{ maxHeight: statusOpen ? '200px' : '0px' }}>
                        <div className="ai-section-body-inner">
                            {isAnalyzing && analysisProgress ? (
                                <div className="ai-status-card">
                                    <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
                                        <span className="ai-status-dot ready" />
                                        <span style={{ color: '#ccc', fontSize: '0.75rem' }}>{providerName}</span>
                                    </div>
                                    <div style={{ color: '#888', fontSize: '0.7rem' }}>
                                        {analysisProgress.scored} / {analysisProgress.total} photos
                                        {' · '}{analysisProgress.phase}
                                    </div>
                                    <div className="ai-progress-bar">
                                        <div
                                            className="ai-progress-fill"
                                            style={{ width: `${(analysisProgress.scored / analysisProgress.total) * 100}%` }}
                                        />
                                    </div>
                                    <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 4 }}>
                                        <span style={{ color: '#666', fontSize: '0.65rem' }}>Elapsed: {formatElapsed(elapsed)}</span>
                                        <button
                                            onClick={handleCancel}
                                            style={{ background: 'none', border: 'none', color: '#ef4444', fontSize: '0.65rem', cursor: 'pointer' }}
                                        >
                                            Cancel
                                        </button>
                                    </div>
                                </div>
                            ) : (
                                <div className="ai-status-card">
                                    <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                                        <span className={`ai-status-dot ${providerReady ? 'ready' : 'warning'}`} />
                                        {providerReady ? (
                                            <span style={{ color: '#ccc', fontSize: '0.75rem' }}>
                                                {providerName} — Ready
                                            </span>
                                        ) : (
                                            <button
                                                onClick={onOpenSettings}
                                                style={{
                                                    background: 'none', border: 'none', color: '#6c63ff',
                                                    fontSize: '0.75rem', cursor: 'pointer', padding: 0,
                                                    textDecoration: 'underline',
                                                }}
                                            >
                                                Configure in Settings
                                            </button>
                                        )}
                                    </div>
                                </div>
                            )}
                        </div>
                    </div>
                </div>

                {/* Tab bar */}
                <div style={{
                    display: 'flex', borderBottom: '1px solid #2a2a3a',
                    margin: '4px 0 0',
                }}>
                    {(['scores', 'rankings', 'faces'] as TabId[]).map(tab => (
                        <button
                            key={tab}
                            onClick={() => setActiveTab(tab)}
                            style={{
                                flex: 1,
                                background: 'none',
                                border: 'none',
                                borderBottom: activeTab === tab ? '2px solid #818cf8' : '2px solid transparent',
                                color: activeTab === tab ? '#c7d2fe' : '#888',
                                fontSize: '0.7rem',
                                fontWeight: activeTab === tab ? 600 : 400,
                                padding: '6px 4px',
                                cursor: 'pointer',
                                textTransform: 'capitalize',
                                transition: 'color 0.15s, border-color 0.15s',
                            }}
                        >
                            {tab === 'faces'
                                ? `Faces${clusters.length > 0 ? ` (${clusters.length})` : ''}`
                                : tab.charAt(0).toUpperCase() + tab.slice(1)}
                        </button>
                    ))}
                </div>

                {/* Tab: Scores */}
                {activeTab === 'scores' && (
                    <div>
                        {/* Stale VLM banner */}
                        {staleVLM && (
                            <div style={{
                                background: '#422006', border: '1px solid #f59e0b',
                                borderRadius: 6, padding: '8px 12px', margin: '8px 0',
                                display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                            }}>
                                <span style={{ color: '#fbbf24', fontSize: 12 }}>
                                    AI scores are outdated. Re-analyze for improved results.
                                </span>
                                <button
                                    onClick={() => {
                                        if (onReanalyze) {
                                            onReanalyze();
                                        }
                                        // TODO: trigger re-analysis if onReanalyze not provided
                                    }}
                                    style={{ color: '#818cf8', background: 'none', border: 'none', cursor: 'pointer', fontSize: 12 }}
                                >
                                    Re-analyze
                                </button>
                            </div>
                        )}

                        {/* Selected Photo scores */}
                        <div className="ai-section" style={{ borderTop: 'none' }}>
                            <div className="ai-section-body" style={{ maxHeight: 'none' }}>
                                <div className="ai-section-body-inner">
                                    {!photoData ? (
                                        <span style={{ color: '#555', fontSize: '0.7rem' }}>
                                            {activePhotoPath ? 'Not analyzed' : 'No photo selected'}
                                        </span>
                                    ) : (
                                        <div className="ai-status-card">
                                            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 10 }}>
                                                <span style={{ color: '#ccc', fontSize: '0.8rem', fontWeight: 500 }}>AI Score</span>
                                                <span className={`ai-overall-score score-${scoreColor(photoData.overallScore * 100)}`}>
                                                    {Math.round(photoData.overallScore * 100)} / 100
                                                </span>
                                            </div>

                                            {/* Sub-score breakdown bars */}
                                            <SubScoreBar
                                                label="Aesthetic"
                                                value={photoData.aestheticScore ?? 0}
                                                color="#a78bfa"
                                            />
                                            <SubScoreBar
                                                label="Sharpness"
                                                value={photoData.sharpnessScore ?? 0}
                                                color="#4ade80"
                                            />
                                            {photoData.faceCount > 0 && (
                                                <>
                                                    <SubScoreBar
                                                        label="Face Quality"
                                                        value={photoData.bestFaceSharpness ?? 0}
                                                        color="#fbbf24"
                                                    />
                                                    <SubScoreBar
                                                        label="Eyes Open"
                                                        value={photoData.eyeOpenness ?? 0}
                                                        color="#60a5fa"
                                                    />
                                                </>
                                            )}

                                            {photoDetections.length > 0 && (
                                                <>
                                                    {/* Best face detection metrics */}
                                                    {(() => {
                                                        // eslint-disable-next-line @typescript-eslint/no-explicit-any
                                                        const best = photoDetections.reduce((a: any, b: any) =>
                                                            (a.eyeSharpness || 0) > (b.eyeSharpness || 0) ? a : b
                                                        );
                                                        const expr = Math.round((best.expression || 0) * 100);
                                                        return (
                                                            <>
                                                                <div className="ai-score-bar-container">
                                                                    <span className="ai-score-label">Eyes Open</span>
                                                                    <span className={`ai-score-value ${best.eyesOpen ? 'score-green' : 'score-red'}`}>
                                                                        {best.eyesOpen ? 'Yes' : 'No'}
                                                                    </span>
                                                                </div>
                                                                {expr > 0 && (
                                                                    <div className="ai-score-bar-container">
                                                                        <span className="ai-score-label">Expression</span>
                                                                        <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                                                                            <div className="ai-score-bar">
                                                                                <div className={`ai-score-bar-fill bar-${scoreColor(expr)}`} style={{ width: `${expr}%` }} />
                                                                            </div>
                                                                            <span className={`ai-score-value score-${scoreColor(expr)}`}>{expr}</span>
                                                                        </div>
                                                                    </div>
                                                                )}
                                                            </>
                                                        );
                                                    })()}
                                                </>
                                            )}

                                            <div className="ai-score-bar-container" style={{ marginTop: 6 }}>
                                                <span className="ai-score-label">Faces</span>
                                                <span className="ai-score-value" style={{ color: '#aaa' }}>
                                                    {photoData.faceCount} detected
                                                </span>
                                            </div>

                                            {/* VLM section */}
                                            {vlmScore && (
                                                <div style={{ marginTop: 10, borderTop: '1px solid #333', paddingTop: 8 }}>
                                                    <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 6 }}>
                                                        <span style={{ color: '#ccc', fontSize: '0.75rem', fontWeight: 500 }}>AI Analysis</span>
                                                        {vlmScore.sceneType && (
                                                            <span style={{
                                                                background: '#3b2d6e', color: '#c4b5fd',
                                                                fontSize: '0.6rem', padding: '1px 6px',
                                                                borderRadius: 10, fontWeight: 500,
                                                            }}>
                                                                {vlmScore.sceneType}
                                                            </span>
                                                        )}
                                                    </div>
                                                    <SubScoreBar
                                                        label="Composition"
                                                        value={(vlmScore.composition ?? 0) / 100}
                                                        color="#818cf8"
                                                    />
                                                    <SubScoreBar
                                                        label="Aesthetic"
                                                        value={(vlmScore.aesthetic ?? 0) / 100}
                                                        color="#a78bfa"
                                                    />
                                                    {vlmScore.explanation && (
                                                        <p style={{ color: '#888', fontSize: '0.65rem', margin: '6px 0 0', lineHeight: 1.4 }}>
                                                            {vlmScore.explanation}
                                                        </p>
                                                    )}
                                                    {vlmScore.issues && (() => {
                                                        let parsed: string[] = [];
                                                        try {
                                                            parsed = JSON.parse(vlmScore.issues) as string[];
                                                        } catch {
                                                            if (typeof vlmScore.issues === 'string' && vlmScore.issues.trim()) {
                                                                parsed = [vlmScore.issues];
                                                            }
                                                        }
                                                        return parsed.length > 0 ? (
                                                            <ul style={{ margin: '4px 0 0', padding: '0 0 0 14px' }}>
                                                                {parsed.map((issue, i) => (
                                                                    <li key={i} style={{ color: '#f59e0b', fontSize: '0.63rem', marginBottom: 2 }}>
                                                                        {issue}
                                                                    </li>
                                                                ))}
                                                            </ul>
                                                        ) : null;
                                                    })()}
                                                </div>
                                            )}
                                        </div>
                                    )}
                                </div>
                            </div>
                        </div>

                        {/* Sort & Filter — remains in Scores tab */}
                        <div className="ai-section">
                            <div
                                className="ai-section-header"
                                onClick={() => setFilterOpen(!filterOpen)}
                                tabIndex={0}
                                onKeyDown={e => e.key === 'Enter' && setFilterOpen(!filterOpen)}
                                role="button"
                                aria-expanded={filterOpen}
                            >
                                <span className="ai-section-title">
                                    Sort & Filter{activeFilterCount > 0 ? ` (${activeFilterCount} active)` : ''}
                                </span>
                                {filterOpen ? <ChevronDown size={14} color="#8b8ba7" /> : <ChevronRight size={14} color="#8b8ba7" />}
                            </div>
                            <div className="ai-section-body" style={{ maxHeight: filterOpen ? '300px' : '0px' }}>
                                <div className="ai-section-body-inner">
                                    <div className="ai-status-card">
                                        {/* Sort by AI Score toggle */}
                                        <div className="ai-filter-row">
                                            <span className="ai-filter-label">Sort by AI Score</span>
                                            <label style={{ position: 'relative', display: 'inline-block', width: 36, height: 20, flexShrink: 0 }}>
                                                <input
                                                    type="checkbox"
                                                    checked={sortByScore}
                                                    onChange={e => { setSortByScore(e.target.checked); onSortByScore(e.target.checked); }}
                                                    style={{ opacity: 0, width: 0, height: 0 }}
                                                />
                                                <span style={{
                                                    position: 'absolute', cursor: 'pointer', top: 0, left: 0, right: 0, bottom: 0,
                                                    backgroundColor: sortByScore ? '#6c63ff' : '#555', borderRadius: 10, transition: 'background-color 0.2s',
                                                }} />
                                                <span style={{
                                                    position: 'absolute', height: 16, width: 16,
                                                    left: sortByScore ? 18 : 2, top: 2,
                                                    backgroundColor: sortByScore ? 'white' : '#999',
                                                    borderRadius: '50%', transition: 'left 0.2s, background-color 0.2s',
                                                }} />
                                            </label>
                                        </div>

                                        {/* Min quality slider */}
                                        <div style={{ marginTop: 8 }}>
                                            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                                                <span className="ai-filter-label">Min quality</span>
                                                <span style={{ color: '#aaa', fontSize: '0.7rem' }}>{minQuality}%</span>
                                            </div>
                                            <input
                                                type="range" min={0} max={100} value={minQuality}
                                                onChange={e => { setMinQuality(+e.target.value); onMinQuality(+e.target.value); }}
                                                style={{ width: '100%' }}
                                            />
                                        </div>

                                        {/* Has faces checkbox */}
                                        <div className="ai-filter-row" style={{ marginTop: 4 }}>
                                            <span className="ai-filter-label">Has faces</span>
                                            <input
                                                type="checkbox" checked={hasFaces}
                                                onChange={e => { setHasFaces(e.target.checked); onHasFacesFilter(e.target.checked); }}
                                            />
                                        </div>

                                        {/* Reset */}
                                        {activeFilterCount > 0 && (
                                            <button
                                                onClick={() => {
                                                    setSortByScore(false); onSortByScore(false);
                                                    setMinQuality(0); onMinQuality(0);
                                                    setHasFaces(false); onHasFacesFilter(false);
                                                    setActiveChips(new Set()); onFilterByFace([]);
                                                }}
                                                style={{ background: 'none', border: 'none', color: '#6c63ff', fontSize: '0.7rem', cursor: 'pointer', marginTop: 6 }}
                                            >
                                                Reset all filters
                                            </button>
                                        )}
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                )}

                {/* Tab: Rankings */}
                {activeTab === 'rankings' && (
                    <div className="ai-section" style={{ borderTop: 'none' }}>
                        <div className="ai-section-body" style={{ maxHeight: 'none' }}>
                            <div className="ai-section-body-inner">
                                {rankings.length === 0 ? (
                                    <span style={{ color: '#555', fontSize: '0.7rem' }}>
                                        No rankings yet. Run AI analysis to compare photos.
                                    </span>
                                ) : (
                                    rankings.map((group, gi) => (
                                        <div key={gi} style={{ marginBottom: 12 }}>
                                            <div style={{ color: '#ccc', fontSize: '0.72rem', fontWeight: 500, marginBottom: 2 }}>
                                                {group.groupLabel}
                                            </div>
                                            {group.explanation && (
                                                <p style={{ color: '#666', fontSize: '0.63rem', margin: '0 0 6px', lineHeight: 1.4 }}>
                                                    {group.explanation}
                                                </p>
                                            )}
                                            {/* eslint-disable-next-line @typescript-eslint/no-explicit-any */}
                                            {(group.rankings as any[]).map((item: any, ri: number) => {
                                                const filename = typeof item.photoPath === 'string'
                                                    ? item.photoPath.split('/').pop() ?? item.photoPath
                                                    : '';
                                                const medalColor =
                                                    item.rank === 1 ? '#f59e0b' :
                                                    item.rank === 2 ? '#94a3b8' :
                                                    item.rank === 3 ? '#78716c' : '#444';
                                                return (
                                                    <div key={ri} style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
                                                        <span style={{
                                                            width: 20, height: 20, borderRadius: '50%',
                                                            background: medalColor, color: '#fff',
                                                            fontSize: '0.6rem', fontWeight: 700,
                                                            display: 'flex', alignItems: 'center', justifyContent: 'center',
                                                            flexShrink: 0,
                                                        }}>
                                                            {item.rank}
                                                        </span>
                                                        <div style={{ flex: 1, overflow: 'hidden' }}>
                                                            <div style={{ color: '#bbb', fontSize: '0.68rem', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                                                {filename}
                                                            </div>
                                                            {item.notes && (
                                                                <div style={{ color: '#666', fontSize: '0.6rem', marginTop: 1 }}>
                                                                    {item.notes}
                                                                </div>
                                                            )}
                                                        </div>
                                                        {item.relativeScore != null && (
                                                            <span style={{ color: '#888', fontSize: '0.6rem', flexShrink: 0 }}>
                                                                {Math.round(item.relativeScore * 100)}
                                                            </span>
                                                        )}
                                                    </div>
                                                );
                                            })}
                                        </div>
                                    ))
                                )}
                            </div>
                        </div>
                    </div>
                )}

                {/* Tab: Faces */}
                {activeTab === 'faces' && (
                    <div className="ai-section" style={{ borderTop: 'none' }}>
                        <div style={{ display: 'flex', justifyContent: 'flex-end', padding: '4px 8px 0' }}>
                            {activeChips.size > 0 && (
                                <button
                                    onClick={() => { setActiveChips(new Set()); onFilterByFace([]); }}
                                    style={{ background: 'none', border: 'none', color: '#6c63ff', fontSize: '0.65rem', cursor: 'pointer' }}
                                >
                                    Clear filter
                                </button>
                            )}
                        </div>
                        <div className="ai-section-body" style={{ maxHeight: 'none' }}>
                            <div className="ai-section-body-inner">
                                {isAnalyzing && clusters.length === 0 ? (
                                    <div style={{ display: 'flex', gap: 8 }}>
                                        {[1, 2, 3].map(i => (
                                            <div key={i} className="ai-skeleton" style={{ width: 36, height: 36, borderRadius: '50%' }} />
                                        ))}
                                    </div>
                                ) : clusters.length === 0 ? (
                                    <span style={{ color: '#555', fontSize: '0.7rem' }}>No people detected</span>
                                ) : (
                                    <div className="face-chips">
                                        {clusters.map((cluster, index) => (
                                            <div
                                                key={cluster.id}
                                                className={`face-chip ${activeChips.has(cluster.id) ? 'active' : ''} ${mergeMode !== null && mergeMode !== cluster.id ? 'merge-target' : ''}`}
                                                onClick={() => handleChipClick(cluster.id)}
                                                onContextMenu={e => handleContextMenu(e, cluster.id)}
                                                style={{ animationDelay: `${index * 30}ms` }}
                                            >
                                                <div className="face-chip-avatar">
                                                    {cluster.thumbnail && (
                                                        <img src={cluster.thumbnail} alt={cluster.label} />
                                                    )}
                                                </div>
                                                <span className="face-chip-count">{cluster.count}</span>
                                                {editingLabel === cluster.id ? (
                                                    <input
                                                        className="face-chip-label-input"
                                                        value={editValue}
                                                        onChange={e => setEditValue(e.target.value)}
                                                        onKeyDown={e => {
                                                            if (e.key === 'Enter') handleRename(cluster.id, editValue);
                                                            if (e.key === 'Escape') setEditingLabel(null);
                                                        }}
                                                        onBlur={() => handleRename(cluster.id, editValue)}
                                                        autoFocus
                                                    />
                                                ) : (
                                                    <span
                                                        className="face-chip-label"
                                                        onClick={e => {
                                                            e.stopPropagation();
                                                            setEditingLabel(cluster.id);
                                                            setEditValue(cluster.label);
                                                        }}
                                                    >
                                                        {cluster.label}
                                                    </span>
                                                )}
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </div>
                        </div>
                    </div>
                )}
            </div>

            {/* Error announcement for screen readers */}
            <div className="ai-sr-only" aria-live="polite" id="ai-error-announce" />

            {/* Context menu */}
            {contextMenu && (
                <div className="ai-context-menu" style={{ left: contextMenu.x, top: contextMenu.y }}>
                    <button
                        className="ai-context-menu-item"
                        onClick={() => {
                            setEditingLabel(contextMenu.clusterId);
                            const c = clusters.find(cl => cl.id === contextMenu.clusterId);
                            setEditValue(c?.label || '');
                            setContextMenu(null);
                        }}
                    >
                        Rename
                    </button>
                    <button
                        className="ai-context-menu-item"
                        onClick={() => { setMergeMode(contextMenu.clusterId); setContextMenu(null); }}
                    >
                        Merge with...
                    </button>
                    <button
                        className="ai-context-menu-item"
                        onClick={() => {
                            HideFaceCluster(contextMenu.clusterId, true).catch((err: unknown) =>
                                console.warn('Hide failed:', err)
                            );
                            setContextMenu(null);
                        }}
                    >
                        Hide from filter
                    </button>
                </div>
            )}
        </div>
    );
}
