import { useState, useEffect } from 'react';
import { FolderOpen, Download, HelpCircle, FileText, Clock, Palette } from 'lucide-react';
import { model } from '../../wailsjs/go/models';
import { GetRecentFolders, SelectExportDirectory, ExportPhotos, OpenLog } from '../../wailsjs/go/app/App';

interface SidebarProps {
    currentDir: string;
    photosCount: number;
    selectedCount: number;
    onOpenFolder: () => void;
    onLoadDir: (dir: string) => void;
    photos: model.Photo[];
    selectedPaths: Set<string>;
    onExportSuccess: (msg: string) => void;
    theme: string;
    onThemeChange: (theme: string) => void;
}

export function Sidebar({
    currentDir,
    photosCount,
    selectedCount,
    onOpenFolder,
    onLoadDir,
    photos,
    selectedPaths,
    onExportSuccess,
    theme,
    onThemeChange
}: SidebarProps) {
    const [recents, setRecents] = useState<string[]>([]);
    const [isExporting, setIsExporting] = useState(false);
    const [showHelp, setShowHelp] = useState(false);

    useEffect(() => {
        loadRecents();
    }, [currentDir]);

    const loadRecents = async () => {
        try {
            const folders = await GetRecentFolders();
            setRecents(folders || []);
        } catch (e) {
            console.error(e);
        }
    };

    const handleExport = async () => {
        if (selectedCount === 0) return;

        try {
            const destDir = await SelectExportDirectory();
            if (!destDir) return;

            setIsExporting(true);
            const selectedPhotos = photos.filter(p => selectedPaths.has(p.Path));
            await ExportPhotos(selectedPhotos, destDir);

            // Reload current dir status to show green checks
            if (currentDir) {
                onLoadDir(currentDir);
            }
            onExportSuccess(`Successfully exported ${selectedCount} photos!`);
        } catch (e) {
            console.error(e);
            alert(`Export failed: ${e}`);
        } finally {
            setIsExporting(false);
        }
    };

    const handleHelp = () => {
        setShowHelp(true);
    };

    return (
        <div className="sidebar">
            <h2>CullSnap</h2>

            <div className="sidebar-group">
                <button className="btn w-full" onClick={onOpenFolder}>
                    <FolderOpen size={18} />
                    Open Folder
                </button>

                {currentDir && (
                    <div className="path mt-4">
                        <h2 className="text-small mb-1">Current Folder</h2>
                        <div className="truncate-path" title={currentDir}>{currentDir}</div>
                    </div>
                )}
            </div>

            <div className="sidebar-group mt-4">
                <h2 className="text-small flex items-center gap-2 mb-1">
                    <Palette size={16} /> Theme
                </h2>
                <select className="theme-switcher" value={theme} onChange={(e) => onThemeChange(e.target.value)}>
                    <option value="dark">Dark</option>
                    <option value="light">Light</option>
                    <option value="matrix">Matrix</option>
                </select>
            </div>

            <div className="sidebar-group mt-6 glass-panel p-4 flex flex-col" style={{ flex: 1, overflow: 'hidden' }}>
                <h2 className="text-small flex items-center gap-2 mb-3" style={{ flexShrink: 0 }}>
                    <Clock size={16} /> Recent Folders
                </h2>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem', overflowY: 'auto', flex: 1, paddingRight: '4px' }}>
                    {recents.map((dir, i) => (
                        <button
                            key={i}
                            className="btn text-small justify-start" style={{ background: 'transparent', padding: '0.25rem' }}
                            onClick={() => onLoadDir(dir)}
                            title={dir}
                        >
                            <div className="truncate-path">{dir.split('/').pop() || dir}</div>
                        </button>
                    ))}
                </div>
            </div>

            <div className="sidebar-bottom mt-auto flex flex-col gap-3">
                <button
                    className="btn btn-primary w-full justify-center"
                    disabled={selectedCount === 0 || isExporting}
                    onClick={handleExport}
                >
                    <Download size={18} />
                    {isExporting ? 'Exporting...' : `Export (${selectedCount})`}
                </button>

                <div className="flex gap-2" style={{ display: 'flex', gap: '0.5rem' }}>
                    <button className="btn w-full justify-center" onClick={OpenLog} title="Open Logs">
                        <FileText size={18} />
                        Logs
                    </button>
                    <button className="btn w-full justify-center" onClick={handleHelp} title="Help">
                        <HelpCircle size={18} />
                        Help
                    </button>
                </div>
            </div>

            {/* Custom Help Modal */}
            {showHelp && (
                <div style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, background: 'rgba(0,0,0,0.5)', zIndex: 9999, display: 'flex', alignItems: 'center', justifyContent: 'center' }} onClick={() => setShowHelp(false)}>
                    <div style={{ background: 'var(--bg-panel)', padding: '32px', borderRadius: '12px', border: '1px solid var(--border-color)', minWidth: '400px', maxWidth: '600px', maxHeight: '80vh', overflowY: 'auto' }} onClick={e => e.stopPropagation()}>
                        <div className="flex justify-between items-center mb-6">
                            <h2 style={{ margin: 0 }}>Help & Information</h2>
                            <button className="btn" onClick={() => setShowHelp(false)}>Close</button>
                        </div>

                        <div className="text-small" style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem', lineHeight: '1.6' }}>
                            <section>
                                <h3 className="mb-2" style={{ color: 'var(--accent)' }}>Getting Started</h3>
                                <p>CullSnap is a high-performance photo culling tool designed to help you quickly review, select, and export your best photos from large folders.</p>
                                <ul style={{ listStyleType: 'disc', paddingLeft: '1.5rem', marginTop: '0.5rem' }}>
                                    <li>Click <strong>Open Folder</strong> to choose a directory containing your images.</li>
                                    <li>CullSnap securely scans the folder without moving any files.</li>
                                    <li>If you have previously reviewed a folder, your selections will automatically be restored.</li>
                                </ul>
                            </section>

                            <section>
                                <h3 className="mb-2" style={{ color: 'var(--accent)' }}>Keyboard Shortcuts</h3>
                                <ul style={{ listStyle: 'none', padding: 0 }}>
                                    <li className="mb-2"><kbd style={{ background: 'var(--bg-sidebar)', padding: '2px 6px', borderRadius: '4px', border: '1px solid var(--border-color)' }}>S</kbd> - Toggle selection tag (Blue Checkmark) for the currently active photo.</li>
                                    <li className="mb-2"><kbd style={{ background: 'var(--bg-sidebar)', padding: '2px 6px', borderRadius: '4px', border: '1px solid var(--border-color)' }}>Arrows</kbd> - Use ← / → or ↑ / ↓ to rapidly navigate backward or forward through your photos.</li>
                                    <li><kbd style={{ background: 'var(--bg-sidebar)', padding: '2px 6px', borderRadius: '4px', border: '1px solid var(--border-color)' }}>Click</kbd> - Click any thumbnail in the left grid to view it in full resolution on the right.</li>
                                </ul>
                            </section>

                            <section>
                                <h3 className="mb-2" style={{ color: 'var(--accent)' }}>Exporting Photos</h3>
                                <p>Once you are finished selecting your favorite shots:</p>
                                <ol style={{ listStyleType: 'decimal', paddingLeft: '1.5rem', marginTop: '0.5rem' }}>
                                    <li>Click the <strong>Export (N)</strong> button at the bottom of the sidebar.</li>
                                    <li>Choose a destination directory (e.g. a "Selects" folder).</li>
                                    <li>CullSnap will safely <strong>copy</strong> the selected photos to the destination.</li>
                                    <li>A green checkmark will appear on exported photos to indicate they have been successfully copied out.</li>
                                </ol>
                            </section>

                            <section>
                                <h3 className="mb-2" style={{ color: 'var(--accent)' }}>System Requirements</h3>
                                <p>CullSnap supports extracting embedded profiles from Canon CR2/CR3 and standard JPEG files for blisteringly fast preview rendering. Keep an eye on the bottom status bar for your computer's live System Resource utilization.</p>
                            </section>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
