import { useState, useEffect } from 'react';
import { X, ExternalLink } from 'lucide-react';
import { GetAboutInfo } from '../../wailsjs/go/app/App';

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

    useEffect(() => {
        GetAboutInfo().then(setAbout).catch(console.error);
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
                        <span style={{ fontSize: '0.85rem', color: 'var(--text-secondary)' }}>
                            {about.version}
                        </span>
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
