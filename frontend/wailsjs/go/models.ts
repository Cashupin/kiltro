export namespace services {
	
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

