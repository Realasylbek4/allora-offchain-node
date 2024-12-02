# Gas and Fees

Allora off-chain nodes submits data to the Allora Network in the form of transactions. These transactions are configured in a per-wallet basis. The wallet configuration is in `config.json` under the `wallet` field.

## Notions

- **Gas**: the amount of computational work required to execute a transaction. It is expressed in gas units, typically denominated in `uallo`.
- **Fees**: the amount of `uallo` paid for a transaction, paid at a specific gas price.

Allora Network implements the [Feemarket](https://github.com/skip-mev/feemarket) module to introduce variability in the gas price in the chain. The offchain node user is highly encouraged to set the gas prices to `auto`, so the node can automatically calculate the gas price based on chain's feemarket-based gas prices, otherwise the node will use a constant gas price which may lead to transaction failure.

## Settings

- `gas` (string): can be set to `auto` or a specific gas value. If set to `auto`, the node will automatically calculate the gas limit based on the estimated gas used by the transactions. Recommended: `auto`.
- `gasAdjustment` (float): is used to adjust the gas limit. Recommended: `1.0-1.2`.
- `gasPrices` (string): can be set to `auto` or a specific gas price. If set to `auto`, the node will automatically calculate the gas price based on chain's feemarket-based gas prices. This is the recommended setting, since feemarket introduces variability in the gas price in the chain. Recommended: `auto`.
- `maxFees` (float): set the max fees that can be paid for a transaction. They are expressed numerically in `uallo`. It is recommended to adjust this value based on experience to optimize results, although a conservative value of `500000` could be a good starting point.
