package routes

import (
	"salonpro-backend/config"
	"salonpro-backend/controllers"
	"salonpro-backend/utils"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	r.Use(cors.New(cors.Config{
	AllowOriginsFunc: func(origin string) bool {
		return true // or write your own validation
	},
	AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
	AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
	AllowCredentials: true,
}))

	r.Use(config.PerformanceLogger())

	auth := r.Group("/auth")
	{
		auth.POST("/register", controllers.Register)
		auth.POST("/login", controllers.Login)

		auth.Use(utils.AuthMiddleware())
		auth.GET("/me", controllers.Me)
	}

	api := r.Group("/api")
	api.Use(utils.AuthMiddleware())
	{
		// Customer routes
		customers := api.Group("/customers")
		{
			customers.POST("", controllers.CreateCustomer)
			customers.GET("", controllers.GetCustomers)
			customers.GET("/:id", controllers.GetCustomer)
			customers.PUT("/:id", controllers.UpdateCustomer)
			customers.DELETE("/:id", controllers.DeleteCustomer)
		}

		// Service routes
		services := api.Group("/services")
		{
			services.POST("", controllers.CreateService)
			services.GET("", controllers.GetServices)
			services.GET("/:id", controllers.GetService)
			services.PUT("/:id", controllers.UpdateService)
			services.DELETE("/:id", controllers.DeleteService)
		}

		// Invoice routes
		invoices := api.Group("/invoices")
		{
			invoices.POST("", controllers.CreateInvoice)
			invoices.GET("", controllers.GetInvoices)
			invoices.GET("/:id", controllers.GetInvoice)
			invoices.PUT("/:id", controllers.UpdateInvoice)
			invoices.DELETE("/:id", controllers.DeleteInvoice)
		}

		//Reports routes
		reportController := controllers.ReportController{}
		api.GET("/reports", reportController.GetReportAnalytics)

		// Dashboard routes
		api.GET("/dashboard", controllers.GetDashboardOverview)

		// Settings routes
		profile := auth.Group("/profile", utils.AuthMiddleware()) // utils.AuthMiddleware()
		{
			profile.GET("", controllers.GetProfile)
			profile.PUT("/update-salon", controllers.UpdateSalonProfile)
			profile.PUT("/update-hours", controllers.UpdateWorkingHours)
			profile.PUT("/update-templates", controllers.UpdateReminderTemplates)
			profile.PUT("/update-notifications", controllers.UpdateNotifications)
			profile.POST("/test-notification", controllers.SendTestNotification)
		}

		employees := api.Group("/employees")
		{
			employees.GET("", controllers.GetEmployees)          // GET /api/employees
			employees.POST("", controllers.AddEmployee)          // POST /api/employees
			employees.PUT("/:id", controllers.UpdateEmployee)    // PUT /api/employees/:id
			employees.DELETE("/:id", controllers.DeleteEmployee) // DELETE /api/employees/:id
		}

	}

	return r
}




