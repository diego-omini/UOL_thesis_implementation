export namespace main {
	
	export class MoneyView {
	    Amount: number;
	    Currency: string;
	
	    static createFrom(source: any = {}) {
	        return new MoneyView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Amount = source["Amount"];
	        this.Currency = source["Currency"];
	    }
	}
	export class AccountSummaryView {
	    AccountID: string;
	    NetLiquidation: MoneyView;
	    CashBalance: MoneyView;
	
	    static createFrom(source: any = {}) {
	        return new AccountSummaryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.AccountID = source["AccountID"];
	        this.NetLiquidation = this.convertValues(source["NetLiquidation"], MoneyView);
	        this.CashBalance = this.convertValues(source["CashBalance"], MoneyView);
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
	export class ConnectionStatusView {
	    Connected: boolean;
	    Message: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionStatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Connected = source["Connected"];
	        this.Message = source["Message"];
	    }
	}
	export class InstrumentView {
	    ContractID: number;
	    Symbol: string;
	    Name: string;
	    AssetClass: string;
	    Exchange: string;
	    Currency: string;
	    Sector: string;
	    Industry: string;
	
	    static createFrom(source: any = {}) {
	        return new InstrumentView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ContractID = source["ContractID"];
	        this.Symbol = source["Symbol"];
	        this.Name = source["Name"];
	        this.AssetClass = source["AssetClass"];
	        this.Exchange = source["Exchange"];
	        this.Currency = source["Currency"];
	        this.Sector = source["Sector"];
	        this.Industry = source["Industry"];
	    }
	}
	
	export class PositionView {
	    Instrument: InstrumentView;
	    Quantity: number;
	    AverageCost: MoneyView;
	    MarketPrice: MoneyView;
	    MarketValue: MoneyView;
	    UnrealizedPnL: MoneyView;
	
	    static createFrom(source: any = {}) {
	        return new PositionView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Instrument = this.convertValues(source["Instrument"], InstrumentView);
	        this.Quantity = source["Quantity"];
	        this.AverageCost = this.convertValues(source["AverageCost"], MoneyView);
	        this.MarketPrice = this.convertValues(source["MarketPrice"], MoneyView);
	        this.MarketValue = this.convertValues(source["MarketValue"], MoneyView);
	        this.UnrealizedPnL = this.convertValues(source["UnrealizedPnL"], MoneyView);
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
	export class PortfolioView {
	    AccountSummary: AccountSummaryView;
	    Positions: PositionView[];
	    Empty: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PortfolioView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.AccountSummary = this.convertValues(source["AccountSummary"], AccountSummaryView);
	        this.Positions = this.convertValues(source["Positions"], PositionView);
	        this.Empty = source["Empty"];
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

