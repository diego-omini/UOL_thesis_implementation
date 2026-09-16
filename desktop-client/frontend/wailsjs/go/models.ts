export namespace main {
	
	export class MoneyView {
	    Amount: number;
	    Currency: string;
	    Available: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MoneyView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Amount = source["Amount"];
	        this.Currency = source["Currency"];
	        this.Available = source["Available"];
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
	export class RatioView {
	    Value: number;
	    Available: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RatioView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Value = source["Value"];
	        this.Available = source["Available"];
	    }
	}
	export class AllocationView {
	    Label: string;
	    MarketValue: MoneyView;
	    Exposure: MoneyView;
	    Weight: RatioView;
	
	    static createFrom(source: any = {}) {
	        return new AllocationView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Label = source["Label"];
	        this.MarketValue = this.convertValues(source["MarketValue"], MoneyView);
	        this.Exposure = this.convertValues(source["Exposure"], MoneyView);
	        this.Weight = this.convertValues(source["Weight"], RatioView);
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
	export class DataWarningView {
	    Code: string;
	    Message: string;
	    Count: number;
	
	    static createFrom(source: any = {}) {
	        return new DataWarningView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Code = source["Code"];
	        this.Message = source["Message"];
	        this.Count = source["Count"];
	    }
	}
	export class ExchangeRateView {
	    Currency: string;
	    BaseCurrency: string;
	    BaseAmountPerUnit: number;
	    Available: boolean;
	    Source: string;
	    AsOf: string;
	
	    static createFrom(source: any = {}) {
	        return new ExchangeRateView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Currency = source["Currency"];
	        this.BaseCurrency = source["BaseCurrency"];
	        this.BaseAmountPerUnit = source["BaseAmountPerUnit"];
	        this.Available = source["Available"];
	        this.Source = source["Source"];
	        this.AsOf = source["AsOf"];
	    }
	}
	export class PositionWeightView {
	    ContractID: number;
	    Symbol: string;
	    Name: string;
	    MarketValue: MoneyView;
	    Weight: RatioView;
	
	    static createFrom(source: any = {}) {
	        return new PositionWeightView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ContractID = source["ContractID"];
	        this.Symbol = source["Symbol"];
	        this.Name = source["Name"];
	        this.MarketValue = this.convertValues(source["MarketValue"], MoneyView);
	        this.Weight = this.convertValues(source["Weight"], RatioView);
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
	export class AnalyticsView {
	    CalculatedAt: string;
	    BaseCurrency: string;
	    NetMarketValue: MoneyView;
	    GrossExposure: MoneyView;
	    TotalUnrealizedPnL: MoneyView;
	    LargestPositions: PositionWeightView[];
	    CurrencyAllocation: AllocationView[];
	    AssetClassAllocation: AllocationView[];
	    SectorAllocation: AllocationView[];
	    TopThreeConcentration: RatioView;
	    HHI: RatioView;
	    ExchangeRates: ExchangeRateView[];
	    Warnings: DataWarningView[];
	
	    static createFrom(source: any = {}) {
	        return new AnalyticsView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.CalculatedAt = source["CalculatedAt"];
	        this.BaseCurrency = source["BaseCurrency"];
	        this.NetMarketValue = this.convertValues(source["NetMarketValue"], MoneyView);
	        this.GrossExposure = this.convertValues(source["GrossExposure"], MoneyView);
	        this.TotalUnrealizedPnL = this.convertValues(source["TotalUnrealizedPnL"], MoneyView);
	        this.LargestPositions = this.convertValues(source["LargestPositions"], PositionWeightView);
	        this.CurrencyAllocation = this.convertValues(source["CurrencyAllocation"], AllocationView);
	        this.AssetClassAllocation = this.convertValues(source["AssetClassAllocation"], AllocationView);
	        this.SectorAllocation = this.convertValues(source["SectorAllocation"], AllocationView);
	        this.TopThreeConcentration = this.convertValues(source["TopThreeConcentration"], RatioView);
	        this.HHI = this.convertValues(source["HHI"], RatioView);
	        this.ExchangeRates = this.convertValues(source["ExchangeRates"], ExchangeRateView);
	        this.Warnings = this.convertValues(source["Warnings"], DataWarningView);
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
	    BaseMarketValue: MoneyView;
	    UnrealizedPnL: MoneyView;
	    BaseUnrealizedPnL: MoneyView;
	    PortfolioWeight: RatioView;
	
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
	        this.BaseMarketValue = this.convertValues(source["BaseMarketValue"], MoneyView);
	        this.UnrealizedPnL = this.convertValues(source["UnrealizedPnL"], MoneyView);
	        this.BaseUnrealizedPnL = this.convertValues(source["BaseUnrealizedPnL"], MoneyView);
	        this.PortfolioWeight = this.convertValues(source["PortfolioWeight"], RatioView);
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
	    Analytics: AnalyticsView;
	    Empty: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PortfolioView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.AccountSummary = this.convertValues(source["AccountSummary"], AccountSummaryView);
	        this.Positions = this.convertValues(source["Positions"], PositionView);
	        this.Analytics = this.convertValues(source["Analytics"], AnalyticsView);
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
	
	
	
	export class SessionStatusView {
	    State: string;
	    GatewayReachable: boolean;
	    Authenticated: boolean;
	    Connected: boolean;
	    Competing: boolean;
	    Paper: boolean;
	    PaperKnown: boolean;
	    Ready: boolean;
	    Message: string;
	
	    static createFrom(source: any = {}) {
	        return new SessionStatusView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.State = source["State"];
	        this.GatewayReachable = source["GatewayReachable"];
	        this.Authenticated = source["Authenticated"];
	        this.Connected = source["Connected"];
	        this.Competing = source["Competing"];
	        this.Paper = source["Paper"];
	        this.PaperKnown = source["PaperKnown"];
	        this.Ready = source["Ready"];
	        this.Message = source["Message"];
	    }
	}

}

