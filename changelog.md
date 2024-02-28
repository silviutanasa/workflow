**Version 1.0.1 - 28/02/2024**
- removed `log` param from `Sequential` constructor function. Instead, introduced decorator pattern and 
logger will be passed via option function, eg. `New(..., WithLogger(logger))`
- introduced Storage option to be able to store step result. This will become handy when you're dealing with
microservice arch. and you need to replay an event/message. In case of an event reply, this functionality allows the 
workflow to skip successful steps and only execute previously failed ones.