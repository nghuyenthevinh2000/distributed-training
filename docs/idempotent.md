# Idempotent operation

f(f(x)) = f(x)

No matter how many times we call a function, a state change logic is guaranteed to be called once. This is often important in scenario where a logic must be performed one time. 

For example, a create logic must be called once else broken by multiple created instance.

https://stackoverflow.com/questions/1077412/what-is-an-idempotent-operation