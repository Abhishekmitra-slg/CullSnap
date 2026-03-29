export namespace app {
	
	export class Contributor {
	    name: string;
	    github: string;
	    role: string;
	    bio: string;
	    avatar: string;
	
	    static createFrom(source: any = {}) {
	        return new Contributor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.github = source["github"];
	        this.role = source["role"];
	        this.bio = source["bio"];
	        this.avatar = source["avatar"];
	    }
	}
	export class AboutInfo {
	    version: string;
	    goVersion: string;
	    wailsVersion: string;
	    sqliteVersion: string;
	    ffmpegVersion: string;
	    license: string;
	    repo: string;
	    contributors: Contributor[];
	
	    static createFrom(source: any = {}) {
	        return new AboutInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.goVersion = source["goVersion"];
	        this.wailsVersion = source["wailsVersion"];
	        this.sqliteVersion = source["sqliteVersion"];
	        this.ffmpegVersion = source["ffmpegVersion"];
	        this.license = source["license"];
	        this.repo = source["repo"];
	        this.contributors = this.convertValues(source["contributors"], Contributor);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SystemProbe {
	    OS: string;
	    Arch: string;
	    CPUs: number;
	    RAMMB: number;
	    FDSoftLimit: number;
	    FFmpegReady: boolean;
	    StorageHint: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemProbe(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.OS = source["OS"];
	        this.Arch = source["Arch"];
	        this.CPUs = source["CPUs"];
	        this.RAMMB = source["RAMMB"];
	        this.FDSoftLimit = source["FDSoftLimit"];
	        this.FFmpegReady = source["FFmpegReady"];
	        this.StorageHint = source["StorageHint"];
	    }
	}
	export class AppConfig {
	    maxConnections: number;
	    thumbnailWorkers: number;
	    scannerWorkers: number;
	    serverIdleTimeoutSec: number;
	    cacheDir: string;
	    autoUpdate: string;
	    useNativeSips: boolean;
	    maxCloudCacheMB: number;
	    probe: SystemProbe;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.maxConnections = source["maxConnections"];
	        this.thumbnailWorkers = source["thumbnailWorkers"];
	        this.scannerWorkers = source["scannerWorkers"];
	        this.serverIdleTimeoutSec = source["serverIdleTimeoutSec"];
	        this.cacheDir = source["cacheDir"];
	        this.autoUpdate = source["autoUpdate"];
	        this.useNativeSips = source["useNativeSips"];
	        this.maxCloudCacheMB = source["maxCloudCacheMB"];
	        this.probe = this.convertValues(source["probe"], SystemProbe);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class DedupStatus {
	    hasDuplicates: boolean;
	    duplicateCount: number;
	    duplicates: model.Photo[];
	
	    static createFrom(source: any = {}) {
	        return new DedupStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hasDuplicates = source["hasDuplicates"];
	        this.duplicateCount = source["duplicateCount"];
	        this.duplicates = this.convertValues(source["duplicates"], model.Photo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DedupeResult {
	    uniquePhotos: model.Photo[];
	    duplicateGroups: model.Photo[][];
	
	    static createFrom(source: any = {}) {
	        return new DedupeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uniquePhotos = this.convertValues(source["uniquePhotos"], model.Photo);
	        this.duplicateGroups = this.convertValues(source["duplicateGroups"], model.Photo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportStats {
	    totalBytes: number;
	    deviceStats: Record<string, number>;
	
	    static createFrom(source: any = {}) {
	        return new ImportStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalBytes = source["totalBytes"];
	        this.deviceStats = source["deviceStats"];
	    }
	}
	export class MirrorStats {
	    totalMB: number;
	
	    static createFrom(source: any = {}) {
	        return new MirrorStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalMB = source["totalMB"];
	    }
	}
	export class PhotoEXIF {
	    camera: string;
	    lens: string;
	    iso: string;
	    aperture: string;
	    shutter: string;
	    dateTaken: string;
	
	    static createFrom(source: any = {}) {
	        return new PhotoEXIF(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.camera = source["camera"];
	        this.lens = source["lens"];
	        this.iso = source["iso"];
	        this.aperture = source["aperture"];
	        this.shutter = source["shutter"];
	        this.dateTaken = source["dateTaken"];
	    }
	}

}

export namespace cloudsource {
	
	export class Album {
	    id: string;
	    title: string;
	    mediaCount: number;
	    coverURL: string;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Album(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.mediaCount = source["mediaCount"];
	        this.coverURL = source["coverURL"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CacheStats {
	    total_bytes: number;
	    album_count: number;
	    limit_bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new CacheStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_bytes = source["total_bytes"];
	        this.album_count = source["album_count"];
	        this.limit_bytes = source["limit_bytes"];
	    }
	}
	export class CachedAlbum {
	    provider_id: string;
	    album_id: string;
	    album_title: string;
	    size_bytes: number;
	    file_count: number;
	    // Go type: time
	    synced_at: any;
	
	    static createFrom(source: any = {}) {
	        return new CachedAlbum(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider_id = source["provider_id"];
	        this.album_id = source["album_id"];
	        this.album_title = source["album_title"];
	        this.size_bytes = source["size_bytes"];
	        this.file_count = source["file_count"];
	        this.synced_at = this.convertValues(source["synced_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CloudSourceStatus {
	    providerID: string;
	    displayName: string;
	    isAvailable: boolean;
	    isConnected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CloudSourceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.providerID = source["providerID"];
	        this.displayName = source["displayName"];
	        this.isAvailable = source["isAvailable"];
	        this.isConnected = source["isConnected"];
	    }
	}

}

export namespace device {
	
	export class Device {
	    name: string;
	    vendorID: string;
	    productID: string;
	    serial: string;
	    // Go type: time
	    detectedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Device(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.vendorID = source["vendorID"];
	        this.productID = source["productID"];
	        this.serial = source["serial"];
	        this.detectedAt = this.convertValues(source["detectedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace model {
	
	export class Photo {
	    Path: string;
	    ThumbnailPath: string;
	    Width: number;
	    Height: number;
	    Size: number;
	    // Go type: time
	    TakenAt: any;
	    IsVideo: boolean;
	    Duration: number;
	    TrimStart: number;
	    TrimEnd: number;
	    isRAW: boolean;
	    rawFormat: string;
	    companionPath: string;
	    isRAWCompanion: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Photo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.ThumbnailPath = source["ThumbnailPath"];
	        this.Width = source["Width"];
	        this.Height = source["Height"];
	        this.Size = source["Size"];
	        this.TakenAt = this.convertValues(source["TakenAt"], null);
	        this.IsVideo = source["IsVideo"];
	        this.Duration = source["Duration"];
	        this.TrimStart = source["TrimStart"];
	        this.TrimEnd = source["TrimEnd"];
	        this.isRAW = source["isRAW"];
	        this.rawFormat = source["rawFormat"];
	        this.companionPath = source["companionPath"];
	        this.isRAWCompanion = source["isRAWCompanion"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

