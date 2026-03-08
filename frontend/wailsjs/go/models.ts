export namespace main {
	
	export class SystemResources {
	    cpu: number;
	    ram: number;
	    diskRead: number;
	    diskWrite: number;
	    netSent: number;
	    netRecv: number;
	
	    static createFrom(source: any = {}) {
	        return new SystemResources(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cpu = source["cpu"];
	        this.ram = source["ram"];
	        this.diskRead = source["diskRead"];
	        this.diskWrite = source["diskWrite"];
	        this.netSent = source["netSent"];
	        this.netRecv = source["netRecv"];
	    }
	}

}

export namespace model {
	
	export class Photo {
	    Path: string;
	    Width: number;
	    Height: number;
	    Size: number;
	    // Go type: time
	    TakenAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Photo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.Width = source["Width"];
	        this.Height = source["Height"];
	        this.Size = source["Size"];
	        this.TakenAt = this.convertValues(source["TakenAt"], null);
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

