---
name: go-clean-ddd-hexagonal
description: Apply proactively when designing Go APIs, microservices, or scalable backend structure with Clean Architecture + DDD + Hexagonal. Triggers on ports and adapters, entities, value objects, aggregates, domain events, repositories, use cases, CQRS, outbox, and bounded contexts. Go-only examples and conventions.
---

# Clean Architecture + DDD + Hexagonal (Go)

Arquitectura backend para Go que combina patrones tácticos de DDD, reglas de dependencia de Clean Architecture y puertos/adaptadores hexagonales para construir servicios mantenibles, testeables y evolutivos.

## Cuándo usar (y cuándo no)

| Use When | Skip When |
|----------|-----------|
| Dominio complejo con muchas reglas | CRUD simple, pocas reglas |
| Sistema de larga vida | Prototipo o MVP temporal |
| Equipo mediano/grande | Equipo muy pequeño |
| Múltiples entradas (HTTP, CLI, eventos) | Un único entrypoint simple |
| Necesidad real de desacoplar infraestructura | Infra fija sin cambios previstos |
| Cobertura de tests alta y sostenida | Scripts internos rápidos |

**Empieza simple y evoluciona.** No todo sistema necesita CQRS completo o Event Sourcing.

## Regla crítica: dependencias hacia adentro

Las dependencias siempre apuntan al núcleo:

```text
infrastructure -> application -> domain
```

Señales de violación:
- `domain` importando `database/sql`, HTTP frameworks, drivers de broker.
- handlers/controladores llamando repos directamente y saltándose casos de uso.
- entidades dependiendo de servicios de aplicación.

Validación rápida: si el dominio no puede correrse en tests sin DB/HTTP reales, tus bordes están mal definidos.

## Árboles de decisión rápidos

### ¿Dónde va este código?

```text
¿Regla de negocio pura, sin I/O?          -> domain/
¿Orquesta dominio + efectos externos?      -> application/
¿Habla con DB, red, broker, framework?     -> infrastructure/
¿Define cómo interactuar (interface)?      -> port (dominio o app)
¿Implementa una interface/port?            -> adapter (infra)
```

### ¿Entidad o Value Object?

```text
¿Tiene identidad estable en el tiempo?      -> Entity
¿Se define solo por atributos?              -> Value Object
¿Comparas "es el mismo"?                   -> Entity (ID)
¿Comparas "vale lo mismo"?                 -> Value Object (valor)
```

### ¿Debe ser otro agregado?

```text
¿Deben ser consistentes en la misma transacción? -> Mismo agregado
¿Pueden ser eventualmente consistentes?           -> Agregados separados
¿Se referencian solo por ID?                      -> Agregados separados
¿El agregado crece demasiado?                     -> Dividir
```

Regla práctica: **una transacción = un agregado**. La consistencia entre agregados se resuelve con eventos de dominio.

## Estructura recomendada en Go

```text
internal/
├── domain/                         # Lógica de negocio (sin dependencias externas)
│   ├── order/
│   │   ├── entity.go              # Aggregate root + entidades hijas
│   │   ├── value_objects.go       # Tipos inmutables
│   │   ├── events.go              # Domain events
│   │   ├── repository.go          # Port dirigido (interface)
│   │   └── services.go            # Servicios de dominio
│   └── shared/
│       └── errors.go
├── application/                    # Casos de uso / orquestación
│   ├── place_order/
│   │   ├── command.go
│   │   ├── handler.go
│   │   └── port.go                # Driver port
│   └── shared/
│       └── unit_of_work.go
├── infrastructure/                 # Adaptadores concretos
│   ├── persistence/
│   ├── messaging/
│   ├── http/
│   ├── di/
│   │   ├── builder.go             # Registro de definitions (sarulabs/di)
│   │   └── modules/               # Módulos de registro por capa
│   └── config/
│       └── env.go
└── cmd/
    └── api/
        └── main.go                # Composition root: build/delete contenedor
```

## Building Blocks DDD

| Pattern | Purpose | Layer | Key Rule |
|---------|---------|-------|----------|
| Entity | Identidad + comportamiento | Domain | Igualdad por ID |
| Value Object | Valor inmutable | Domain | Igualdad estructural |
| Aggregate | Frontera de consistencia | Domain | Solo el root se referencia fuera |
| Domain Event | Hecho de negocio ocurrido | Domain | Nombre en pasado |
| Repository | Abstracción de persistencia | Domain port | Por agregado, no por tabla |
| Domain Service | Lógica sin estado | Domain | Solo si no encaja en entidad |
| Application Service | Orquestación de casos de uso | Application | Coordina dominio + puertos |

## Anti-patrones críticos

| Anti-Pattern | Problema | Fix |
|--------------|----------|-----|
| Anemic Domain Model | Entidades sin comportamiento | Llevar reglas a entidades/agregados |
| Repository per entity/table | Rompe límites de agregado | Un repo por agregado |
| Infra leakage | Dominio acoplado a DB/HTTP | Dominio puro |
| God aggregate | Contención y complejidad | Partir agregado |
| Saltar puertos | Acoplamiento alto | Siempre pasar por application |
| CRUD thinking | Modelado centrado en tablas | Modelar comportamiento |
| CQRS prematuro | Complejidad injustificada | Empezar simple |
| TX cross-aggregate | Bloqueos y acoplamiento | Eventos + consistencia eventual |

## Orden de implementación

1. Descubrir dominio (event storming, lenguaje ubicuo).
2. Modelar entidades, value objects y agregados (sin infraestructura).
3. Definir puertos (repositorios y servicios externos).
4. Implementar casos de uso.
5. Añadir adaptadores (HTTP, DB, broker) al final.

## Convenciones Go que aportan calidad

- Aceptar `context.Context` en puertos de app/infra.
- Errores de dominio tipados + `errors.Is`/`errors.As`.
- Interfaces pequeñas, preferiblemente definidas cerca del consumidor.
- Composición explícita en `cmd/.../main.go` o `wire.go`.
- Si usas `sarulabs/di`, registra definitions por módulo en `infrastructure/di/modules` y evita resolver dependencias fuera de composition root/adapters.
- Tests table-driven en dominio y casos de uso.

## Referencias

| File | Purpose |
|------|---------|
| [references/LAYERS.md](references/LAYERS.md) | Especificación por capas en Go |
| [references/DDD-STRATEGIC.md](references/DDD-STRATEGIC.md) | Bounded contexts y context mapping |
| [references/DDD-TACTICAL.md](references/DDD-TACTICAL.md) | Entidades, VOs, agregados y repos |
| [references/HEXAGONAL.md](references/HEXAGONAL.md) | Puertos/adaptadores en Go + DI con sarulabs/di |
| [references/CQRS-EVENTS.md](references/CQRS-EVENTS.md) | CQRS, eventos y outbox |
| [references/TESTING.md](references/TESTING.md) | Estrategia de tests |
| [references/CHEATSHEET.md](references/CHEATSHEET.md) | Guía rápida de decisiones |

## Sources

### Primary Sources
- [The Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html) — Robert C. Martin (2012)
- [Hexagonal Architecture](https://alistair.cockburn.us/hexagonal-architecture/) — Alistair Cockburn (2005)
- [Domain-Driven Design: The Blue Book](https://www.domainlanguage.com/ddd/blue-book/) — Eric Evans (2003)
- [Implementing Domain-Driven Design](https://openlibrary.org/works/OL17392277W) — Vaughn Vernon (2013)

### Pattern References
- [CQRS](https://martinfowler.com/bliki/CQRS.html) — Martin Fowler
- [Event Sourcing](https://martinfowler.com/eaaDev/EventSourcing.html) — Martin Fowler
- [Repository Pattern](https://martinfowler.com/eaaCatalog/repository.html) — Martin Fowler
- [Unit of Work](https://martinfowler.com/eaaCatalog/unitOfWork.html) — Martin Fowler
- [Bounded Context](https://martinfowler.com/bliki/BoundedContext.html) — Martin Fowler
- [Transactional Outbox](https://microservices.io/patterns/data/transactional-outbox.html) — microservices.io
- [Effective Aggregate Design](https://www.dddcommunity.org/library/vernon_2011/) — Vaughn Vernon
