export namespace ibkr {
	
	export class ConnectionStatus {
	    Connected: boolean;
	    Message: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Connected = source["Connected"];
	        this.Message = source["Message"];
	    }
	}

}

