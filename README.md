# Workflow

A library that allows you to define the business logic in steps and orchestrate them in a workflow.

**Sequential** is a workflow that runs all of its steps/commands in a predefined order/sequence. \
It has the ability to retry at the step level, with a configured number of attempts and delay. \
It allows optional steps to run as post workflow execution hooks, which means they'll run no mather the status of the workflow execution(success or error)