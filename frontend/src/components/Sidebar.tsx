import { useState, useEffect } from 'react';
import { FolderOpen, Download, HelpCircle, FileText, Clock, Palette, Layers, X } from 'lucide-react';
import { model } from '../../wailsjs/go/models';
import { GetRecentFolders, SelectExportDirectory, ExportPhotos, OpenLog } from '../../wailsjs/go/app/App';

interface SidebarProps {
    currentDir: string;
    photosCount: number;
    selectedCount: number;
    onOpenFolder: () => void;
    onLoadDir: (dir: string) => void;
    onDeduplicate: () => void;
    isDeduplicating: boolean;
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
    onDeduplicate,
    isDeduplicating,
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
            if (currentDir) onLoadDir(currentDir);
            onExportSuccess(`Successfully exported ${selectedCount} photos!`);
        } catch (e) {
            console.error(e);
            alert(`Export failed: ${e}`);
        } finally {
            setIsExporting(false);
        }
    };

    return (
        <div className="sidebar">
            <div className="sidebar-logo">CullSnap</div>

            {/* Actions */}
            <div className="sidebar-group">
                <button className="btn btn-gradient w-full" onClick={onOpenFolder}>
                    <FolderOpen size={16} />
                    Open Folder
                </button>

                {currentDir && (
                    <button
                        className="btn w-full mt-2"
                        onClick={onDeduplicate}
                        disabled={isDeduplicating || photosCount === 0}
                        title="Find and group duplicate photos"
                    >
                        <Layers size={16} />
                        {isDeduplicating ? 'Processing...' : 'Find Duplicates'}
                    </button>
                )}
            </div>

            {/* Current folder */}
            {currentDir && (
                <div style={{ padding: '0 4px' }}>
                    <div className="sidebar-label">Current Folder</div>
                    <div className="truncate-path text-small" title={currentDir} style={{ padding: '0 12px' }}>
                        {currentDir}
                    </div>
                </div>
            )}

            {/* Theme */}
            <div style={{ padding: '0 4px' }}>
                <div className="sidebar-label">
                    <Palette size={12} style={{ display: 'inline', verticalAlign: 'middle', marginRight: 4 }} />
                    Theme
                </div>
                <select
                    className="theme-switcher"
                    value={theme}
                    onChange={(e) => onThemeChange(e.target.value)}
                >
                    <option value="dark">Dark</option>
                    <option value="light">Light</option>
                </select>
            </div>

            {/* Recents */}
            <div className="recents-panel" style={{ marginTop: 8 }}>
                <div className="sidebar-label" style={{ padding: '0 0 8px 0' }}>
                    <Clock size={12} style={{ display: 'inline', verticalAlign: 'middle', marginRight: 4 }} />
                    Recent Folders
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 2, overflowY: 'auto', flex: 1 }}>
                    {recents.map((dir, i) => (
                        <button
                            key={i}
                            className="recent-item"
                            onClick={() => onLoadDir(dir)}
                            title={dir}
                        >
                            <span className="truncate-path">{dir.split('/').pop() || dir}</span>
                        </button>
                    ))}
                    {recents.length === 0 && (
                        <span className="text-small" style={{ padding: '4px 8px', color: 'var(--text-muted)' }}>
                            No recent folders
                        </span>
                    )}
                </div>
            </div>

            {/* Bottom actions */}
            <div className="mt-auto flex flex-col gap-2">
                <button
                    className="btn btn-gradient w-full justify-center"
                    disabled={selectedCount === 0 || isExporting}
                    onClick={handleExport}
                >
                    <Download size={16} />
                    {isExporting ? 'Exporting...' : `Export (${selectedCount})`}
                </button>

                <div style={{ display: 'flex', gap: 6 }}>
                    <button className="btn w-full justify-center" onClick={OpenLog} title="Open Logs">
                        <FileText size={14} />
                        Logs
                    </button>
                    <button className="btn w-full justify-center" onClick={() => setShowHelp(true)} title="Help">
                        <HelpCircle size={14} />
                        Help
                    </button>
                </div>
            </div>

            {/* Help Modal */}
            {showHelp && (
                <div className="modal-overlay" onClick={() => setShowHelp(false)}>
                    <div className="modal-content" onClick={e => e.stopPropagation()}>
                        <div className="flex justify-between items-center mb-3">
                            <h2 style={{ margin: 0 }}>Help & Shortcuts</h2>
                            <button className="btn" onClick={() => setShowHelp(false)} style={{ padding: '4px 8px' }}>
                                <X size={16} />
                            </button>
                        </div>

                        <div className="text-small" style={{ display: 'flex', flexDirection: 'column', gap: '1.25rem', lineHeight: '1.6' }}>
                            <section>
                                <h3 style={{ color: 'var(--accent)', fontSize: '0.8125rem', marginBottom: 6 }}>Getting Started</h3>
                                <p>CullSnap is a high-performance photo culling tool. Click <strong>Open Folder</strong> to load a directory of images. Navigate, select, and export your best shots.</p>
                            </section>

                            <section>
                                <h3 style={{ color: 'var(--accent)', fontSize: '0.8125rem', marginBottom: 6 }}>Keyboard Shortcuts</h3>
                                <ul style={{ listStyle: 'none', padding: 0, display: 'flex', flexDirection: 'column', gap: 6 }}>
                                    <li><kbd style={{ background: 'var(--bg-panel)', padding: '2px 6px', borderRadius: 4, border: '1px solid var(--border-color)', fontSize: '0.7rem' }}>S</kbd> — Toggle selection on active photo</li>
                                    <li><kbd style={{ background: 'var(--bg-panel)', padding: '2px 6px', borderRadius: 4, border: '1px solid var(--border-color)', fontSize: '0.7rem' }}>← →</kbd> — Navigate between photos</li>
                                    <li><kbd style={{ background: 'var(--bg-panel)', padding: '2px 6px', borderRadius: 4, border: '1px solid var(--border-color)', fontSize: '0.7rem' }}>Click</kbd> — View photo full-size</li>
                                </ul>
                            </section>

                            <section>
                                <h3 style={{ color: 'var(--accent)', fontSize: '0.8125rem', marginBottom: 6 }}>Exporting</h3>
                                <p>Select photos with <strong>S</strong>, then click <strong>Export</strong> to copy them to a destination folder. Exported photos show a green badge.</p>
                            </section>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
