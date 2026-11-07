export namespace config {
	
	export class Settings {
	    php?: string;
	    ssl?: boolean;
	    httpPort: number;
	    httpsPort: number;
	    mysqlPort: number;
	    redisPort: number;
	    phpBasePort: number;
	    phpWorkers: number;
	    lan?: boolean;
	    www?: string;
	    lang?: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.php = source["php"];
	        this.ssl = source["ssl"];
	        this.httpPort = source["httpPort"];
	        this.httpsPort = source["httpsPort"];
	        this.mysqlPort = source["mysqlPort"];
	        this.redisPort = source["redisPort"];
	        this.phpBasePort = source["phpBasePort"];
	        this.phpWorkers = source["phpWorkers"];
	        this.lan = source["lan"];
	        this.www = source["www"];
	        this.lang = source["lang"];
	    }
	}

}

export namespace logview {
	
	export class Log {
	    id: string;
	    label: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new Log(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.path = source["path"];
	    }
	}
	export class Result {
	    lines: string[];
	    size: number;
	    exists: boolean;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lines = source["lines"];
	        this.size = source["size"];
	        this.exists = source["exists"];
	        this.truncated = source["truncated"];
	    }
	}

}

export namespace main {
	
	export class I18nView {
	    lang: string;
	    pref: string;
	    supported: string[];
	    messages: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new I18nView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.lang = source["lang"];
	        this.pref = source["pref"];
	        this.supported = source["supported"];
	        this.messages = source["messages"];
	    }
	}
	export class SettingsView {
	    settings: config.Settings;
	    defaultWww: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settings = this.convertValues(source["settings"], config.Settings);
	        this.defaultWww = source["defaultWww"];
	        this.error = source["error"];
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
	export class PHPVersion {
	    name: string;
	    version: string;
	    nts: boolean;
	    selected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PHPVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	        this.nts = source["nts"];
	        this.selected = source["selected"];
	    }
	}
	export class SSLStatus {
	    enabled: boolean;
	    available: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new SSLStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.available = source["available"];
	        this.error = source["error"];
	    }
	}
	export class ShimsStatus {
	    dir: string;
	    inPath: boolean;
	    php: string;
	    composer: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ShimsStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dir = source["dir"];
	        this.inPath = source["inPath"];
	        this.php = source["php"];
	        this.composer = source["composer"];
	        this.error = source["error"];
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

