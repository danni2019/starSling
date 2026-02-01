import logging
from py_vollib import black_scholes, black
from py_vollib.black.implied_volatility import implied_volatility
from py_vollib.black.greeks.analytical import delta, gamma, vega, theta

"""
py-vollib==1.0.1

black-scholes: stock option with no dividend paid. Spot S + r

black: using future price as "current price" and discount option yield with e^{-r t}, Future F + r
        all cases and function under this model are using discounted option price

black-scholes-merton: stock option with dividend considered. Spot S + r,q
"""

PARAM_DAYS_IN_YEAR = 365


class OptionModel:
    def __init__(self):
        pass

    """
    Calculate future option price and implied volatility using the Black model.
    """
    @classmethod
    def future_option_theo_price(cls, option_type: str, underlying: float, strike: float, tte: float, r: float, sigma: float) -> float | None:
        """
        Calculate the theoretical future option price using the Black model.
        
        Args:
            - option_type: Type of the option ('c' or 'p' for call/put)
            - underlying: Current price of the underlying asset
            - strike: Strike price of the option
            - tte: Time to expiration (in days)
            - r: Risk-free interest rate (annualized)
            - sigma: Volatility of the underlying asset (annualized)

        """
        try:
            t = tte / PARAM_DAYS_IN_YEAR  # convert days to years
            return black.black(option_type, underlying, strike, t, r, sigma)
        except Exception as e:
            logging.getLogger(__name__).error(
                "OptionModel.future_option_theo_price failed: params=(option_type=%s, underlying=%s, strike=%s, tte=%s, r=%s, sigma=%s) error=%s: %s",
                option_type,
                underlying,
                strike,
                tte,
                r,
                sigma,
                e.__class__.__name__,
                str(e),
            )
            return None
    
    @classmethod
    def future_option_imp_vol(cls, option_type: str, underlying: float, strike: float, tte: float, r: float, option_price: float) -> float | None:
        """
        Calculate the implied volatility of a future option using the Black model.

        Args:
            - option_type: Type of the option ('c' or 'p' for call/put)
            - underlying: Current price of the underlying asset
            - strike: Strike price of the option
            - tte: Time to expiration (in days)
            - r: Risk-free interest rate (annualized)
            - option_price: Market price of the option

        """
        try:
            t = tte / PARAM_DAYS_IN_YEAR  # convert days to years
            return implied_volatility(option_price, underlying, strike, r, t, option_type)
        except Exception as e:
            logging.getLogger(__name__).error(
                "OptionModel.future_option_imp_vol failed: params=(option_type=%s, underlying=%s, strike=%s, tte=%s, r=%s, option_price=%s) error=%s: %s",
                option_type,
                underlying,
                strike,
                tte,
                r,
                option_price,
                e.__class__.__name__,
                str(e),
            )
            return None

    @classmethod
    def future_option_delta(cls, option_type: str, underlying: float, strike: float, tte: float, r: float, sigma: float) -> float | None:
        """
        Calculate the delta of a future option using the Black model.

        Args:
            - option_type: Type of the option ('c' or 'p' for call/put)
            - underlying: Current price of the underlying asset
            - strike: Strike price of the option
            - tte: Time to expiration (in days)
            - r: Risk-free interest rate (annualized)
            - sigma: Volatility of the underlying asset (annualized)

        """
        try:
            t = tte / PARAM_DAYS_IN_YEAR  # convert days to years
            return delta(option_type, underlying, strike, t, r, sigma)
        except Exception as e:
            logging.getLogger(__name__).error(
                "OptionModel.future_option_delta failed: params=(option_type=%s, underlying=%s, strike=%s, tte=%s, r=%s, sigma=%s) error=%s: %s",
                option_type,
                underlying,
                strike,
                tte,
                r,
                sigma,
                e.__class__.__name__,
                str(e),
            )
            return None

    @classmethod
    def future_option_gamma(cls, option_type: str, underlying: float, strike: float, tte: float, r: float, sigma: float) -> float | None:
        """
        Calculate the delta of a future option using the Black model.

        Args:
            - option_type: Type of the option ('c' or 'p' for call/put)
            - underlying: Current price of the underlying asset
            - strike: Strike price of the option
            - tte: Time to expiration (in days)
            - r: Risk-free interest rate (annualized)
            - sigma: Volatility of the underlying asset (annualized)

        """
        try:
            t = tte / PARAM_DAYS_IN_YEAR  # convert days to years
            return gamma(option_type, underlying, strike, t, r, sigma)
        except Exception as e:
            logging.getLogger(__name__).error(
                "OptionModel.future_option_gamma failed: params=(option_type=%s, underlying=%s, strike=%s, tte=%s, r=%s, sigma=%s) error=%s: %s",
                option_type,
                underlying,
                strike,
                tte,
                r,
                sigma,
                e.__class__.__name__,
                str(e),
            )
            return None 
    
    @classmethod
    def future_option_theta(cls, option_type: str, underlying: float, strike: float, tte: float, r: float, sigma: float) -> float | None:
        """
        Calculate the delta of a future option using the Black model.

        Args:
            - option_type: Type of the option ('c' or 'p' for call/put)
            - underlying: Current price of the underlying asset
            - strike: Strike price of the option
            - tte: Time to expiration (in days)
            - r: Risk-free interest rate (annualized)
            - sigma: Volatility of the underlying asset (annualized)

        """
        try:
            t = tte / PARAM_DAYS_IN_YEAR  # convert days to years
            return theta(option_type, underlying, strike, t, r, sigma)
        except Exception as e:
            logging.getLogger(__name__).error(
                "OptionModel.future_option_theta failed: params=(option_type=%s, underlying=%s, strike=%s, tte=%s, r=%s, sigma=%s) error=%s: %s",
                option_type,
                underlying,
                strike,
                tte,
                r,
                sigma,
                e.__class__.__name__,
                str(e),
            )
            return None
    
    @classmethod
    def future_option_vega(cls, option_type: str, underlying: float, strike: float, tte: float, r: float, sigma: float) -> float | None:
        """
        Calculate the delta of a future option using the Black model.

        Args:
            - option_type: Type of the option ('c' or 'p' for call/put)
            - underlying: Current price of the underlying asset
            - strike: Strike price of the option
            - tte: Time to expiration (in days)
            - r: Risk-free interest rate (annualized)
            - sigma: Volatility of the underlying asset (annualized)

        """
        try:
            t = tte / PARAM_DAYS_IN_YEAR  # convert days to years
            return vega(option_type, underlying, strike, t, r, sigma)
        except Exception as e:
            logging.getLogger(__name__).error(
                "OptionModel.future_option_vega failed: params=(option_type=%s, underlying=%s, strike=%s, tte=%s, r=%s, sigma=%s) error=%s: %s",
                option_type,
                underlying,
                strike,
                tte,
                r,
                sigma,
                e.__class__.__name__,
                str(e),
            )
            return None
