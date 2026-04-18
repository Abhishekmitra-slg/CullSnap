import { useRef, useState, useEffect } from 'react';

interface ThumbProgress {
    current: number;
    total: number;
    heicCount?: number;
    heicDecoder?: string;
}

interface SysMetrics {
    cpu: number;
    ram: number;
    diskRead: number;
    diskWrite: number;
    netSent: number;
    netRecv: number;
}

interface VLMStatus {
    state: string;
    modelName: string;
    backend: string;
}

export interface BottomBarProps {
    photoCount: number;
    selectedCount: number;
    loading: boolean;
    thumbProgress?: ThumbProgress | null;
    sysMetrics?: SysMetrics | null;
    vlmStatus?: VLMStatus | null;
    onVLMClick?: () => void;
}

function vlmColor(state: string): string {
    switch (state) {
        case 'ready':
            return '#4ade80';
        case 'starting':
        case 'busy':
            return '#fbbf24';
        case 'error':
        case 'crashed':
            return '#ef4444';
        default:
            return '#9ca3af';
    }
}

function vlmLabel(state: string, modelName: string, backend: string, width: number): string {
    if (width > 900) {
        let label = `AI: ${state}`;
        if (modelName) {
            label += ` (${modelName}`;
            if (backend) label += ` - ${backend}`;
            label += ')';
        }
        return label;
    }
    if (width > 600) {
        return `AI: ${state}${modelName ? ' - Model Active' : ''}`;
    }
    return state;
}

export function BottomBar({
    photoCount,
    selectedCount,
    loading,
    thumbProgress,
    sysMetrics,
    vlmStatus,
    onVLMClick,
}: BottomBarProps) {
    const containerRef = useRef<HTMLDivElement>(null);
    const [barWidth, setBarWidth] = useState(window.innerWidth);

    useEffect(() => {
        const el = containerRef.current;
        if (!el) return;
        const ro = new ResizeObserver((entries) => {
            for (const entry of entries) {
                setBarWidth(entry.contentRect.width);
            }
        });
        ro.observe(el);
        return () => ro.disconnect();
    }, []);

    return (
        <div
            ref={containerRef}
            className="status-bar"
            style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                width: '100%',
                paddingRight: '1rem',
            }}
        >
            <div style={{ flex: 1 }}>
                {loading ? (
                    <div className="progress-container">
                        <div className="progress-bar-indeterminate"></div>
                    </div>
                ) : (
                    <span>
                        {photoCount} photos &bull; {selectedCount} selected
                        {thumbProgress
                            ? ` \u2022 Loading thumbnails ${thumbProgress.current}/${thumbProgress.total}`
                            : ''}
                        {thumbProgress &&
                        thumbProgress.heicCount &&
                        thumbProgress.heicCount > 0 ? (
                            <span
                                style={{
                                    color:
                                        thumbProgress.heicDecoder === 'sips'
                                            ? '#a78bfa'
                                            : '#d4a017',
                                    marginLeft: '6px',
                                }}
                            >
                                {thumbProgress.heicDecoder === 'sips'
                                    ? `${thumbProgress.heicCount} HEIC via native decoder`
                                    : `${thumbProgress.heicCount} HEIC via FFmpeg (slower)`}
                            </span>
                        ) : null}
                    </span>
                )}
            </div>

            {sysMetrics && (
                <div
                    className="sys-metrics flex items-center gap-3"
                    style={{ background: 'transparent', border: 'none', boxShadow: 'none' }}
                >
                    <span title="CPU Usage">CPU: {sysMetrics.cpu.toFixed(1)}%</span>
                    <span title="Backend Engine Memory Usage">
                        Engine RAM: {sysMetrics.ram.toFixed(0)} MB
                    </span>
                    <span title="Disk Read/Write">
                        Disk: {(sysMetrics.diskRead || 0).toFixed(1)}/
                        {(sysMetrics.diskWrite || 0).toFixed(1)} MB/s
                    </span>
                    <span title="Network Send/Recv">
                        Net: {(sysMetrics.netSent || 0).toFixed(1)}/
                        {(sysMetrics.netRecv || 0).toFixed(1)} KB/s
                    </span>
                </div>
            )}

            {vlmStatus && vlmStatus.state !== 'off' && (
                <span
                    role="button"
                    tabIndex={0}
                    onClick={onVLMClick}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') onVLMClick?.();
                    }}
                    style={{
                        marginLeft: 12,
                        color: vlmColor(vlmStatus.state),
                        cursor: onVLMClick ? 'pointer' : 'default',
                        userSelect: 'none',
                    }}
                >
                    {vlmLabel(vlmStatus.state, vlmStatus.modelName, vlmStatus.backend, barWidth)}
                </span>
            )}
        </div>
    );
}
