export namespace main {
	
	export class CounselorLimitationView {
	    Statement: string;
	    FactIDs: string[];
	
	    static createFrom(source: any = {}) {
	        return new CounselorLimitationView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Statement = source["Statement"];
	        this.FactIDs = source["FactIDs"];
	    }
	}
	export class CitationView {
	    Title: string;
	    URL: string;
	
	    static createFrom(source: any = {}) {
	        return new CitationView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Title = source["Title"];
	        this.URL = source["URL"];
	    }
	}
	export class ReportedValueView {
	    FactID: string;
	    Value: number;
	    Unit: string;
	
	    static createFrom(source: any = {}) {
	        return new ReportedValueView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.FactID = source["FactID"];
	        this.Value = source["Value"];
	        this.Unit = source["Unit"];
	    }
	}
	export class CounselorObservationView {
	    Category: string;
	    Statement: string;
	    SupportingFactIDs: string[];
	    ReportedValues: ReportedValueView[];
	    Severity: string;
	    Confidence: string;
	    Limitations: string[];
	    Explanation: string;
	    Citations: CitationView[];
	
	    static createFrom(source: any = {}) {
	        return new CounselorObservationView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Category = source["Category"];
	        this.Statement = source["Statement"];
	        this.SupportingFactIDs = source["SupportingFactIDs"];
	        this.ReportedValues = this.convertValues(source["ReportedValues"], ReportedValueView);
	        this.Severity = source["Severity"];
	        this.Confidence = source["Confidence"];
	        this.Limitations = source["Limitations"];
	        this.Explanation = source["Explanation"];
	        this.Citations = this.convertValues(source["Citations"], CitationView);
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
	export class CounselorResponseView {
	    SchemaVersion: number;
	    Summary: string;
	    Observations: CounselorObservationView[];
	    Limitations: CounselorLimitationView[];
	
	    static createFrom(source: any = {}) {
	        return new CounselorResponseView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.SchemaVersion = source["SchemaVersion"];
	        this.Summary = source["Summary"];
	        this.Observations = this.convertValues(source["Observations"], CounselorObservationView);
	        this.Limitations = this.convertValues(source["Limitations"], CounselorLimitationView);
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
	export class AIResponseView {
	    Text: string;
	    Structured: CounselorResponseView;
	    HasStructured: boolean;
	    Provider: string;
	    Model: string;
	    PromptID: string;
	    PromptVersion: number;
	
	    static createFrom(source: any = {}) {
	        return new AIResponseView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Text = source["Text"];
	        this.Structured = this.convertValues(source["Structured"], CounselorResponseView);
	        this.HasStructured = source["HasStructured"];
	        this.Provider = source["Provider"];
	        this.Model = source["Model"];
	        this.PromptID = source["PromptID"];
	        this.PromptVersion = source["PromptVersion"];
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
	export class FundLookThroughView {
	    FundSymbol: string;
	    FundName: string;
	    Available: boolean;
	    Complete: boolean;
	    SourceURL: string;
	    SourceType: string;
	    RetrievedAt: string;
	    ExpiresAt: string;
	    EffectiveDate: string;
	    CoveragePercentage: number;
	    CoverageAvailable: boolean;
	    TopHoldingsOnly: boolean;
	    CurrencyAssumptions: string[];
	    ClassificationAssumptions: string[];
	    Warnings: string[];
	    Confidence: string;
	
	    static createFrom(source: any = {}) {
	        return new FundLookThroughView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.FundSymbol = source["FundSymbol"];
	        this.FundName = source["FundName"];
	        this.Available = source["Available"];
	        this.Complete = source["Complete"];
	        this.SourceURL = source["SourceURL"];
	        this.SourceType = source["SourceType"];
	        this.RetrievedAt = source["RetrievedAt"];
	        this.ExpiresAt = source["ExpiresAt"];
	        this.EffectiveDate = source["EffectiveDate"];
	        this.CoveragePercentage = source["CoveragePercentage"];
	        this.CoverageAvailable = source["CoverageAvailable"];
	        this.TopHoldingsOnly = source["TopHoldingsOnly"];
	        this.CurrencyAssumptions = source["CurrencyAssumptions"];
	        this.ClassificationAssumptions = source["ClassificationAssumptions"];
	        this.Warnings = source["Warnings"];
	        this.Confidence = source["Confidence"];
	    }
	}
	export class EffectiveExposureView {
	    MetricID: string;
	    SecurityID: string;
	    Symbol: string;
	    Name: string;
	    Sector: string;
	    MarketValue: MoneyView;
	    Exposure: MoneyView;
	    Weight: RatioView;
	
	    static createFrom(source: any = {}) {
	        return new EffectiveExposureView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.MetricID = source["MetricID"];
	        this.SecurityID = source["SecurityID"];
	        this.Symbol = source["Symbol"];
	        this.Name = source["Name"];
	        this.Sector = source["Sector"];
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
	    EffectiveSecurityExposure: EffectiveExposureView[];
	    EffectiveSectorAllocation: AllocationView[];
	    FundLookThrough: FundLookThroughView[];
	    EffectiveExposureComplete: boolean;
	
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
	        this.EffectiveSecurityExposure = this.convertValues(source["EffectiveSecurityExposure"], EffectiveExposureView);
	        this.EffectiveSectorAllocation = this.convertValues(source["EffectiveSectorAllocation"], AllocationView);
	        this.FundLookThrough = this.convertValues(source["FundLookThrough"], FundLookThroughView);
	        this.EffectiveExposureComplete = source["EffectiveExposureComplete"];
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
	
	
	
	
	
	
	
	export class InstrumentSearchResultView {
	    ContractID: number;
	    Symbol: string;
	    Name: string;
	    Exchange: string;
	    AssetClass: string;
	
	    static createFrom(source: any = {}) {
	        return new InstrumentSearchResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ContractID = source["ContractID"];
	        this.Symbol = source["Symbol"];
	        this.Name = source["Name"];
	        this.Exchange = source["Exchange"];
	        this.AssetClass = source["AssetClass"];
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
	
	export class OptionalNumberView {
	    Value: number;
	    Available: boolean;
	    UnavailableReason: string;
	
	    static createFrom(source: any = {}) {
	        return new OptionalNumberView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Value = source["Value"];
	        this.Available = source["Available"];
	        this.UnavailableReason = source["UnavailableReason"];
	    }
	}
	export class OrderPreviewView {
	    Token: string;
	    ExpiresAt: string;
	    Instrument: InstrumentView;
	    Side: string;
	    Quantity: number;
	    EstimatedPrice: MoneyView;
	    OrderAmount: MoneyView;
	    Commission: MoneyView;
	    Total: MoneyView;
	    CashBalance: MoneyView;
	    CurrentPosition: number;
	    ResultingPosition: number;
	    Warnings: string[];
	
	    static createFrom(source: any = {}) {
	        return new OrderPreviewView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Token = source["Token"];
	        this.ExpiresAt = source["ExpiresAt"];
	        this.Instrument = this.convertValues(source["Instrument"], InstrumentView);
	        this.Side = source["Side"];
	        this.Quantity = source["Quantity"];
	        this.EstimatedPrice = this.convertValues(source["EstimatedPrice"], MoneyView);
	        this.OrderAmount = this.convertValues(source["OrderAmount"], MoneyView);
	        this.Commission = this.convertValues(source["Commission"], MoneyView);
	        this.Total = this.convertValues(source["Total"], MoneyView);
	        this.CashBalance = this.convertValues(source["CashBalance"], MoneyView);
	        this.CurrentPosition = source["CurrentPosition"];
	        this.ResultingPosition = source["ResultingPosition"];
	        this.Warnings = source["Warnings"];
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
	export class OrderResultView {
	    OrderID: string;
	    Status: string;
	    WarningToken: string;
	    WarningMessages: string[];
	
	    static createFrom(source: any = {}) {
	        return new OrderResultView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.OrderID = source["OrderID"];
	        this.Status = source["Status"];
	        this.WarningToken = source["WarningToken"];
	        this.WarningMessages = source["WarningMessages"];
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
	export class StockHistoryPointView {
	    Time: string;
	    Open: number;
	    High: number;
	    Low: number;
	    Close: number;
	    Volume: number;
	
	    static createFrom(source: any = {}) {
	        return new StockHistoryPointView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Time = source["Time"];
	        this.Open = source["Open"];
	        this.High = source["High"];
	        this.Low = source["Low"];
	        this.Close = source["Close"];
	        this.Volume = source["Volume"];
	    }
	}
	export class StockHistoryView {
	    Range: string;
	    Points: StockHistoryPointView[];
	    MarketDataStatus: string;
	    DelaySeconds: number;
	    UnavailableReason: string;
	
	    static createFrom(source: any = {}) {
	        return new StockHistoryView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Range = source["Range"];
	        this.Points = this.convertValues(source["Points"], StockHistoryPointView);
	        this.MarketDataStatus = source["MarketDataStatus"];
	        this.DelaySeconds = source["DelaySeconds"];
	        this.UnavailableReason = source["UnavailableReason"];
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
	export class StockSnapshotView {
	    Instrument: InstrumentView;
	    Currency: string;
	    OpenPrice: OptionalNumberView;
	    ClosePrice: OptionalNumberView;
	    BidPrice: OptionalNumberView;
	    AskPrice: OptionalNumberView;
	    Volume: OptionalNumberView;
	    High52Week: OptionalNumberView;
	    Low52Week: OptionalNumberView;
	    MarketDataStatus: string;
	    AsOf: string;
	
	    static createFrom(source: any = {}) {
	        return new StockSnapshotView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Instrument = this.convertValues(source["Instrument"], InstrumentView);
	        this.Currency = source["Currency"];
	        this.OpenPrice = this.convertValues(source["OpenPrice"], OptionalNumberView);
	        this.ClosePrice = this.convertValues(source["ClosePrice"], OptionalNumberView);
	        this.BidPrice = this.convertValues(source["BidPrice"], OptionalNumberView);
	        this.AskPrice = this.convertValues(source["AskPrice"], OptionalNumberView);
	        this.Volume = this.convertValues(source["Volume"], OptionalNumberView);
	        this.High52Week = this.convertValues(source["High52Week"], OptionalNumberView);
	        this.Low52Week = this.convertValues(source["Low52Week"], OptionalNumberView);
	        this.MarketDataStatus = source["MarketDataStatus"];
	        this.AsOf = source["AsOf"];
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

