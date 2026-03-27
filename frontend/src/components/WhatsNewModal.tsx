import { useState, useEffect } from 'react';
import { X, Sparkles } from 'lucide-react';
import { GetChangelogForCurrentVersion, AcknowledgeWhatsNew } from '../../wailsjs/go/app/App';

interface WhatsNewModalProps {
    onClose: () => void;
}

interface ChangelogSection {
    title: string;
    items: string[];
}

function parseChangelog(raw: string): { version: string; date: string; sections: ChangelogSection[] } {
    const lines = raw.split('\n');
    let version = '';
    let date = '';
    const sections: ChangelogSection[] = [];
    let currentSection: ChangelogSection | null = null;

    for (const line of lines) {
        const versionMatch = line.match(/^## \[(.+?)\] - (.+)$/);
        if (versionMatch) {
            version = versionMatch[1];
            date = versionMatch[2];
            continue;
        }
        const sectionMatch = line.match(/^### (.+)$/);
        if (sectionMatch) {
            if (currentSection && currentSection.items.length > 0) {
                sections.push(currentSection);
            }
            currentSection = { title: sectionMatch[1], items: [] };
            continue;
        }
        if (line.startsWith('- ') && currentSection) {
            let item = line.slice(2);
            item = item.replace(/\*\(([^)]+)\)\*\s*/, '[$1] ');
            currentSection.items.push(item);
        }
    }
    if (currentSection && currentSection.items.length > 0) {
        sections.push(currentSection);
    }
    return { version, date, sections };
}

const sectionIcons: Record<string, string> = {
    'Security': '\u{1F6E1}',
    'Features': '\u{2728}',
    'Bug Fixes': '\u{1F41B}',
    'Documentation': '\u{1F4DD}',
    'Performance': '\u{26A1}',
    'Miscellaneous': '\u{1F527}',
    'Breaking Changes': '\u{1F4A5}',
};

export function WhatsNewModal({ onClose }: WhatsNewModalProps) {
    const [changelog, setChangelog] = useState<{ version: string; date: string; sections: ChangelogSection[] } | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        GetChangelogForCurrentVersion()
            .then(raw => {
                if (raw) {
                    setChangelog(parseChangelog(raw));
                }
                setLoading(false);
            })
            .catch(() => setLoading(false));
    }, []);

    const handleDismiss = () => {
        AcknowledgeWhatsNew().catch(console.error);
        onClose();
    };

    useEffect(() => {
        if (!loading && (!changelog || changelog.sections.length === 0)) {
            AcknowledgeWhatsNew().catch(console.error);
            onClose();
        }
    }, [loading, changelog, onClose]);

    if (loading) return null;
    if (!changelog || changelog.sections.length === 0) return null;

    return (
        <div className="settings-overlay" onClick={handleDismiss}>
            <div
                className="settings-modal glass-panel"
                onClick={e => e.stopPropagation()}
                style={{ maxWidth: 480, maxHeight: '70vh', display: 'flex', flexDirection: 'column' }}
            >
                <div className="settings-header">
                    <h2 style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                        <Sparkles size={18} />
                        What's New in {changelog.version}
                    </h2>
                    <button className="btn icon-btn" onClick={handleDismiss}><X size={16} /></button>
                </div>
                <div style={{ fontSize: '0.7rem', color: 'var(--text-secondary)', marginBottom: 12 }}>
                    Released {changelog.date}
                </div>
                <div style={{ flex: 1, overflowY: 'auto', paddingRight: 8 }}>
                    {changelog.sections.map(section => (
                        <div key={section.title} style={{ marginBottom: 16 }}>
                            <h3 style={{ fontSize: '0.8rem', fontWeight: 600, marginBottom: 6, color: 'var(--text-primary)' }}>
                                {sectionIcons[section.title] || ''} {section.title}
                            </h3>
                            <ul style={{ margin: 0, paddingLeft: 20, fontSize: '0.75rem', lineHeight: 1.6 }}>
                                {section.items.map((item, i) => (
                                    <li key={i} style={{ color: 'var(--text-secondary)', marginBottom: 4 }}>
                                        {item}
                                    </li>
                                ))}
                            </ul>
                        </div>
                    ))}
                </div>
                <div style={{ borderTop: '1px solid var(--border-color)', paddingTop: 12, marginTop: 8, textAlign: 'right' }}>
                    <button className="btn" onClick={handleDismiss} style={{ padding: '6px 20px', fontSize: '0.75rem' }}>
                        Dismiss
                    </button>
                </div>
            </div>
        </div>
    );
}
