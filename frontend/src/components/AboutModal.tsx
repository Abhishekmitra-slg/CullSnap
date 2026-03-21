import { useState, useEffect, useRef } from 'react';
import { X, ExternalLink, Loader } from 'lucide-react';
import { GetAboutInfo, CheckForUpdate } from '../../wailsjs/go/app/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

interface Contributor {
    name: string;
    github: string;
    role: string;
    bio: string;
    avatar: string;
}

interface AboutData {
    version: string;
    goVersion: string;
    wailsVersion: string;
    sqliteVersion: string;
    ffmpegVersion: string;
    license: string;
    repo: string;
    contributors: Contributor[];
}

interface AboutModalProps {
    onClose: () => void;
}

export function AboutModal({ onClose }: AboutModalProps) {
    const [about, setAbout] = useState<AboutData | null>(null);
    const [checking, setChecking] = useState(false);
    const [checkResult, setCheckResult] = useState<string | null>(null);
    const checkTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

    useEffect(() => {
        GetAboutInfo().then(setAbout).catch(console.error);
    }, []);

    const handleCheckForUpdate = () => {
        setChecking(true);
        setCheckResult(null);

        EventsOn('update:available', () => {
            setChecking(false);
            setCheckResult(null); // Toast will handle it
            EventsOff('update:available');
            EventsOff('update:error');
            if (checkTimerRef.current) clearTimeout(checkTimerRef.current);
        });

        EventsOn('update:error', (data: { message: string }) => {
            setChecking(false);
            setCheckResult(data.message);
            EventsOff('update:available');
            EventsOff('update:error');
            if (checkTimerRef.current) clearTimeout(checkTimerRef.current);
        });

        // Fallback: if no events fire within 8s, assume up to date
        checkTimerRef.current = setTimeout(() => {
            setChecking(false);
            setCheckResult('You\'re up to date!');
            EventsOff('update:available');
            EventsOff('update:error');
        }, 8000);

        CheckForUpdate().catch(err => {
            setChecking(false);
            setCheckResult(`Check failed: ${err}`);
            EventsOff('update:available');
            EventsOff('update:error');
            if (checkTimerRef.current) clearTimeout(checkTimerRef.current);
        });
    };

    useEffect(() => {
        return () => {
            if (checkTimerRef.current) clearTimeout(checkTimerRef.current);
            EventsOff('update:available');
            EventsOff('update:error');
        };
    }, []);

    if (!about) {
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
            <div className="settings-modal glass-panel" onClick={e => e.stopPropagation()} style={{ maxWidth: 520 }}>
                <div className="settings-header">
                    <h2>About CullSnap</h2>
                    <button className="btn icon-btn" onClick={onClose}><X size={16} /></button>
                </div>

                {/* App Info */}
                <section className="settings-section">
                    <div style={{ textAlign: 'center', marginBottom: 16 }}>
                        <h3 style={{ fontSize: '1.25rem', margin: '0 0 4px' }}>CullSnap</h3>
                        <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', margin: '0 0 4px', fontStyle: 'italic' }}>Fast photo culling, simplified.</p>
                        <span style={{ fontSize: '0.85rem', color: 'var(--text-secondary)' }}>
                            {about.version}
                        </span>
                        <div style={{ marginTop: 8 }}>
                            <button
                                className="btn outline"
                                style={{ fontSize: '0.75rem', padding: '4px 12px', display: 'inline-flex', alignItems: 'center', gap: 6 }}
                                onClick={handleCheckForUpdate}
                                disabled={checking}
                            >
                                {checking && <Loader size={12} className="spin" />}
                                {checking ? 'Checking…' : 'Check for Updates'}
                            </button>
                            {checkResult && (
                                <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', margin: '6px 0 0' }}>
                                    {checkResult}
                                </p>
                            )}
                        </div>
                    </div>
                    <div style={{ display: 'flex', gap: 12, justifyContent: 'center', marginBottom: 8 }}>
                        <a
                            href={about.repo}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="btn outline"
                            style={{ fontSize: '0.75rem', padding: '4px 12px', display: 'flex', alignItems: 'center', gap: 4 }}
                        >
                            <ExternalLink size={12} /> GitHub
                        </a>
                        <span className="btn outline" style={{ fontSize: '0.75rem', padding: '4px 12px', cursor: 'default' }}>
                            {about.license}
                        </span>
                    </div>
                </section>

                {/* Tech Stack */}
                <section className="settings-section">
                    <h3>Tech Stack</h3>
                    <div className="settings-info-grid">
                        <span>Go</span><span>{about.goVersion}</span>
                        <span>Wails</span><span>{about.wailsVersion}</span>
                        <span>SQLite</span><span>{about.sqliteVersion}</span>
                        <span>FFmpeg</span><span>{about.ffmpegVersion}</span>
                    </div>
                </section>

                {/* Contributors */}
                <section className="settings-section">
                    <h3>Contributors</h3>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                        {about.contributors.map((c) => (
                            <div
                                key={c.github}
                                style={{
                                    display: 'flex',
                                    gap: 12,
                                    alignItems: 'flex-start',
                                    padding: 10,
                                    borderRadius: 8,
                                    background: 'rgba(255,255,255,0.03)',
                                }}
                            >
                                <img
                                    src={c.avatar}
                                    alt={c.name}
                                    style={{ width: 40, height: 40, borderRadius: '50%', flexShrink: 0 }}
                                />
                                <div style={{ minWidth: 0 }}>
                                    <div style={{ display: 'flex', alignItems: 'center', gap: 6, flexWrap: 'wrap' }}>
                                        <a
                                            href={`https://github.com/${c.github}`}
                                            target="_blank"
                                            rel="noopener noreferrer"
                                            style={{ fontWeight: 600, fontSize: '0.85rem', color: 'var(--text-primary)', textDecoration: 'none' }}
                                        >
                                            {c.name}
                                        </a>
                                        <span style={{
                                            fontSize: '0.65rem',
                                            padding: '1px 6px',
                                            borderRadius: 4,
                                            background: 'rgba(139,92,246,0.15)',
                                            color: 'rgb(167,139,250)',
                                        }}>
                                            {c.role}
                                        </span>
                                    </div>
                                    <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', margin: '4px 0 0', lineHeight: 1.4 }}>
                                        {c.bio}
                                    </p>
                                </div>
                            </div>
                        ))}
                    </div>
                </section>
            </div>
        </div>
    );
}
