# :SIG(...) Grammar (Draft)

Scope: This defines the grammar for the contents of `:SIG(...)` only.

## Summary

Two forms are supported:

- Type form: `T`
- Function form: `Args -> Ret`

Examples:

```
# :SIG(App::cpm)
# :SIG(array[any])
# :SIG((App::cpm, any) -> void)
# :SIG(any -> any)
# :SIG((any, any) -> (any, any))

# For variables:
# my $x; # :SIG(array[int])  => arrayref[int]
# my @x; # :SIG(array[int])  => array[int]
# my %x; # :SIG(hash[int])   => hash[int]
```

## EBNF

```
Sig        = FuncType | Type ;

FuncType   = Args , WS? , "->" , WS? , Ret ;

Args       = VoidArg
           | SingleArg
           | MultiArgs ;

VoidArg    = "void" | "(void)" ;
SingleArg  = Type | "(" , WS? , Type , WS? , ")" ;
MultiArgs  = "(" , WS? , TypeList , WS? , ")" ;

Ret        = VoidRet | SingleRet | MultiRet ;

VoidRet    = "void" | "(void)" ;
SingleRet  = Type | "(" , WS? , Type , WS? , ")" ;
MultiRet   = "(" , WS? , TypeList , WS? , ")" ;

TypeList   = Type , { WS? , "," , WS? , Type } ;

Type       = SimpleType | ContainerType ;

SimpleType = "any" | "int" | "undef" | ClassName ;

ContainerType = "array" , "[" , WS? , Type , WS? , "]"
              | "hash"  , "[" , WS? , Type , WS? , "]" ;

ClassName  = Ident , { "::" , Ident } ;

Ident      = Alpha , { Alpha | Digit | "_" } ;

WS         = { " " | "\t" | "\n" | "\r" } ;
Alpha      = "A".."Z" | "a".."z" | "_" ;
Digit      = "0".."9" ;
```

## Notes

- `void` means "no arguments" or "no return values" depending on context.
- Parentheses are optional for 0 or 1 arguments/returns, and required for 2+.
- `array[T]` and `hash[T]` describe container types whose element/value type is `T`.
  - For scalar vars (`$x`), these are interpreted as references (arrayref/hashref).
  - For list vars (`@x`/`%x`), these are interpreted as non-ref containers.
- `ClassName` is any Perl package name (e.g., `Foo`, `Foo::Bar`).
- No union/intersection/optional types are in scope yet.
