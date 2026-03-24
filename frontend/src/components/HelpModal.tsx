import { useState } from 'react';
import { X } from 'lucide-react';

interface HelpModalProps {
    onClose: () => void;
}

const tabs = ['Getting Started', 'Shortcuts', 'Features', 'Exporting'] as const;
type Tab = (typeof tabs)[number];

function Kbd({ children }: { children: React.ReactNode }) {
    return (
        <kbd
            style={{
                background: 'var(--bg-surface)',
                padding: '2px 8px',
                borderRadius: 4,
                border: '1px solid var(--border-color)',
                fontSize: '0.7rem',
                fontFamily: 'inherit',
                fontWeight: 600,
                minWidth: 24,
                display: 'inline-block',
                textAlign: 'center',
            }}
        >
            {children}
        </kbd>
    );
}

function ShortcutRow({ keys, description }: { keys: React.ReactNode; description: string }) {
    return (
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '6px 0' }}>
            <div style={{ minWidth: 100, display: 'flex', gap: 4, flexShrink: 0 }}>{keys}</div>
            <span style={{ color: 'var(--text-secondary)', fontSize: '0.8rem' }}>{description}</span>
        </div>
    );
}

function GettingStartedTab() {
    const steps = [
        {
            title: '1. Open a folder',
            text: 'Click Open Folder in the sidebar or drag a folder into the window to load your photos, RAW files, and videos.',
        },
        {
            title: '2. Browse & review',
            text: 'Use arrow keys or click thumbnails to navigate. The viewer shows the full-resolution image with EXIF details.',
        },
        {
            title: '3. Select your keepers',
            text: 'Press S or click the checkmark on thumbnails to select photos you want to keep. Use star ratings (1\u20135) to rank quality.',
        },
        {
            title: '4. Find duplicates',
            text: 'Click Find Duplicates to automatically detect near-identical photos using perceptual hashing. Duplicates are grouped and highlighted in the grid.',
        },
        {
            title: '5. Export',
            text: 'Click Export to copy your selected photos to a new folder. Video trim points are preserved during export.',
        },
    ];

    return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <p style={{ color: 'var(--text-secondary)', fontSize: '0.8rem', margin: 0, lineHeight: 1.6 }}>
                CullSnap helps you quickly sort through large photo collections. Here's the typical workflow:
            </p>
            {steps.map((step) => (
                <div key={step.title}>
                    <div style={{ fontWeight: 600, fontSize: '0.825rem', color: 'var(--text-primary)', marginBottom: 4 }}>
                        {step.title}
                    </div>
                    <div style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                        {step.text}
                    </div>
                </div>
            ))}
        </div>
    );
}

function ShortcutsTab() {
    return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div>
                <h4 style={{ margin: '0 0 8px', fontSize: '0.78rem', color: 'var(--accent)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                    Navigation
                </h4>
                <ShortcutRow
                    keys={<><Kbd>&larr;</Kbd><Kbd>&uarr;</Kbd></>}
                    description="Previous photo"
                />
                <ShortcutRow
                    keys={<><Kbd>&rarr;</Kbd><Kbd>&darr;</Kbd></>}
                    description="Next photo"
                />
            </div>
            <div>
                <h4 style={{ margin: '0 0 8px', fontSize: '0.78rem', color: 'var(--accent)', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
                    Selection & Rating
                </h4>
                <ShortcutRow
                    keys={<Kbd>S</Kbd>}
                    description="Toggle selection on active photo"
                />
                <ShortcutRow
                    keys={<><Kbd>1</Kbd> <span style={{ color: 'var(--text-muted)', fontSize: '0.7rem' }}>to</span> <Kbd>5</Kbd></>}
                    description="Set star rating (press again to clear)"
                />
            </div>
        </div>
    );
}

function FeaturesTab() {
    const features = [
        {
            title: 'Smart Deduplication',
            text: 'Uses perceptual hashing (dHash) to find visually similar photos, even if they differ in resolution or compression. Photos are scored by quality to help you pick the best version.',
        },
        {
            title: 'Star Ratings',
            text: 'Rate photos from 1 to 5 stars using keyboard shortcuts or by clicking stars on thumbnails. Ratings persist across sessions.',
        },
        {
            title: 'RAW Image Support',
            text: 'Native Pure Go support for 11 camera RAW formats: CR2, CR3, ARW, NEF, DNG, RAF, RW2, ORF, NRW, PEF, SRW. All formats are handled natively with zero external dependencies. RAW files show format badges in the grid and viewer. RAW+JPEG pairs shot simultaneously are automatically linked.',
        },
        {
            title: 'Video Support',
            text: 'Browse and preview video files alongside photos. Set trim points in the viewer to export only the portion you need. Requires FFmpeg.',
        },
        {
            title: 'Themes',
            text: 'Toggle between dark and light mode using the sun/moon icon in the sidebar header.',
        },
        {
            title: 'System Metrics',
            text: 'The status bar shows real-time CPU, RAM, disk I/O, and network usage so you can monitor performance during large scans.',
        },
        {
            title: 'HEIC/HEIF Support',
            text: 'Native support for iPhone HEIC/HEIF photos. On macOS, uses the built-in sips decoder for fast hardware-accelerated conversion. Falls back to FFmpeg on Windows and Linux. You can switch between decoders in Settings > HEIC Decoder.',
        },
        {
            title: 'Cloud Albums',
            text: "Browse and cull photos from Google Drive and iCloud without manual downloads. Click 'Cloud Albums' in the sidebar to connect your cloud storage. Photos are mirrored locally for fast culling, and your selections persist across sessions.",
        },
        {
            title: 'Import from Device (macOS)',
            text: "Connect an iPhone or iPad via USB and CullSnap will detect it automatically. Click 'Import from Device' in the sidebar or use the auto-detect notification to import photos for culling.",
        },
    ];

    return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            {features.map((f) => (
                <div key={f.title}>
                    <div style={{ fontWeight: 600, fontSize: '0.825rem', color: 'var(--text-primary)', marginBottom: 4 }}>
                        {f.title}
                    </div>
                    <div style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                        {f.text}
                    </div>
                </div>
            ))}
        </div>
    );
}

function ExportingTab() {
    return (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <p style={{ color: 'var(--text-secondary)', fontSize: '0.8rem', margin: 0, lineHeight: 1.6 }}>
                Export copies your selected photos, RAW files, and videos to a new folder, keeping originals untouched. RAW+JPEG companions are exported independently — select each file you want to keep.
            </p>

            <div>
                <div style={{ fontWeight: 600, fontSize: '0.825rem', color: 'var(--text-primary)', marginBottom: 4 }}>
                    How to export
                </div>
                <ol style={{ margin: 0, paddingLeft: 20, fontSize: '0.78rem', color: 'var(--text-secondary)', lineHeight: 1.8 }}>
                    <li>Select the photos you want to keep (press <strong>S</strong> or click checkmarks)</li>
                    <li>Click the <strong>Export</strong> button in the sidebar</li>
                    <li>Choose a destination folder</li>
                    <li>Name your export session (a default timestamp name is provided)</li>
                </ol>
            </div>

            <div>
                <div style={{ fontWeight: 600, fontSize: '0.825rem', color: 'var(--text-primary)', marginBottom: 4 }}>
                    Video trimming
                </div>
                <div style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                    If you set trim points on a video in the viewer, the exported file will contain only the trimmed segment. This requires FFmpeg to be installed on your system.
                </div>
            </div>

            <div>
                <div style={{ fontWeight: 600, fontSize: '0.825rem', color: 'var(--text-primary)', marginBottom: 4 }}>
                    Supported formats
                </div>
                <div style={{ fontSize: '0.78rem', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                    Images: JPG, JPEG, PNG, HEIC, HEIF, CR2, CR3, ARW, NEF, DNG, RAF, RW2, ORF, NRW, PEF, SRW. Videos: MP4, MOV, AVI, MKV, WEBM (when FFmpeg is available).
                </div>
            </div>
        </div>
    );
}

export function HelpModal({ onClose }: HelpModalProps) {
    const [activeTab, setActiveTab] = useState<Tab>('Getting Started');

    return (
        <div className="settings-overlay" onClick={onClose}>
            <div className="settings-modal glass-panel" onClick={e => e.stopPropagation()} style={{ maxWidth: 540 }}>
                <div className="settings-header">
                    <h2>Help</h2>
                    <button className="btn icon-btn" onClick={onClose}><X size={16} /></button>
                </div>

                {/* Tab bar */}
                <div
                    style={{
                        display: 'flex',
                        gap: 2,
                        borderBottom: '1px solid var(--border-color)',
                        paddingBottom: 0,
                        marginBottom: 4,
                    }}
                >
                    {tabs.map((tab) => (
                        <button
                            key={tab}
                            onClick={() => setActiveTab(tab)}
                            style={{
                                background: 'none',
                                border: 'none',
                                borderBottom: activeTab === tab ? '2px solid var(--accent)' : '2px solid transparent',
                                padding: '8px 14px',
                                fontSize: '0.75rem',
                                fontWeight: activeTab === tab ? 600 : 400,
                                color: activeTab === tab ? 'var(--text-primary)' : 'var(--text-secondary)',
                                cursor: 'pointer',
                                transition: 'color 0.15s, border-color 0.15s',
                            }}
                        >
                            {tab}
                        </button>
                    ))}
                </div>

                {/* Tab content */}
                <section className="settings-section" style={{ flex: 1, overflowY: 'auto' }}>
                    {activeTab === 'Getting Started' && <GettingStartedTab />}
                    {activeTab === 'Shortcuts' && <ShortcutsTab />}
                    {activeTab === 'Features' && <FeaturesTab />}
                    {activeTab === 'Exporting' && <ExportingTab />}
                </section>
            </div>
        </div>
    );
}
