export namespace main {
	
	export class SitesResult {
	    sites: vhosts.Site[];
	    skipped: vhosts.Skipped[];
	
	    static createFrom(source: any = {}) {
	        return new SitesResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sites = this.convertValues(source["sites"], vhosts.Site);
	        this.skipped = this.convertValues(source["skipped"], vhosts.Skipped);
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

export namespace services {
	
	export class HostsStatus {
	    upToDate: boolean;
	    domains: string[];
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new HostsStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.upToDate = source["upToDate"];
	        this.domains = source["domains"];
	        this.error = source["error"];
	    }
	}
	export class Info {
	    name: string;
	    status: string;
	    error: string;
	    ports: number[];
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.ports = source["ports"];
	        this.detail = source["detail"];
	    }
	}

}

export namespace vhosts {
	
	export class Site {
	    name: string;
	    domain: string;
	    folder: string;
	    root: string;
	    laravel: boolean;
	    tool: boolean;
	    localOnly: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Site(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.domain = source["domain"];
	        this.folder = source["folder"];
	        this.root = source["root"];
	        this.laravel = source["laravel"];
	        this.tool = source["tool"];
	        this.localOnly = source["localOnly"];
	    }
	}
	export class Skipped {
	    folder: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new Skipped(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.folder = source["folder"];
	        this.reason = source["reason"];
	    }
	}

}

